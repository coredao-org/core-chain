package feemarket

import (
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/log"
)

const (
	// ChainHeadChanSize Size of the channel for chain head events
	ChainHeadChanSize = 32

	// cacheCleanupInterval is the interval at which the cache is checked for stale entries
	cacheCleanupInterval = 5 * time.Minute

	// cacheEntryBlockThreshold is the number of blocks to keep in the cache
	cacheEntryBlockThreshold = 14400 // 12 hours

	// cacheEntryExpiration is the expiration time for a cache entry
	cacheEntryExpiration = 12 * time.Hour
)

// MiningWorkID is a new type for work identification
type MiningWorkID struct {
	ParentHash common.Hash
	Timestamp  uint64
	AttemptNum uint64
}

// CacheMetadata is a cache entry base struct
type CacheMetadata struct {
	blockNum  uint64
	blockHash common.Hash
	modified  time.Time
}

// ConstantsEntry keeps track of the constants
type ConstantsEntry struct {
	CacheMetadata
	constants types.FeeMarketConstants
}

// MiningConstantsCache is a temporary cache for mining operations
type MiningConstantsCache struct {
	entry  *ConstantsEntry
	workID MiningWorkID
}

// ConfigEntry is a cache entry for a fee market config
type ConfigEntry struct {
	CacheMetadata
	config types.FeeMarketConfig
}

// MiningConfigsCache is a temporary cache for mining operations
type MiningConfigsCache struct {
	entries map[common.Address]*ConfigEntry
	workID  MiningWorkID
}

// FeeMarketCache is a cache for fee market configs
type FeeMarketCache struct {
	constants        *ConstantsEntry
	tempConstantsMap map[MiningWorkID]*MiningConstantsCache

	entries        map[common.Address]*ConfigEntry
	tempEntriesMap map[MiningWorkID]*MiningConfigsCache

	head       common.Hash
	headHeight uint64
	parentHash common.Hash

	chainSub event.Subscription
	chainCh  chan ChainEvent
	closeCh  chan struct{}

	lock sync.RWMutex
}

func NewFeeMarketCache(reader BlockChain) *FeeMarketCache {
	currentHeader := reader.CurrentHeader()

	c := &FeeMarketCache{
		tempConstantsMap: make(map[MiningWorkID]*MiningConstantsCache),
		entries:          make(map[common.Address]*ConfigEntry),
		tempEntriesMap:   make(map[MiningWorkID]*MiningConfigsCache),
		head:             currentHeader.Hash(),
		headHeight:       currentHeader.Number.Uint64(),
		parentHash:       currentHeader.ParentHash,

		chainCh: make(chan ChainEvent, ChainHeadChanSize),
		closeCh: make(chan struct{}),
	}

	// Subscribe to chain events
	c.chainSub = reader.FeeMarketSubscribeChainEvent(c.chainCh)

	go c.loop()
	go c.removeStaleEntries()

	return c
}

// loop is the main loop for the storage provider
func (c *FeeMarketCache) loop() {
	defer c.chainSub.Unsubscribe()

	for {
		select {
		case event := <-c.chainCh:
			c.onChainEvent(event)
		case <-c.closeCh:
			return
		}
	}
}

// onChainEvent is called when a new chain event is received and it updates the cache head
func (c *FeeMarketCache) onChainEvent(event ChainEvent) {
	c.lock.Lock()
	defer c.lock.Unlock()

	newHead := event.Hash
	newHeight := event.Block.Number.Uint64()

	isReorg := event.Block.ParentHash != c.head
	if isReorg {
		// Clear the constants as they're now invalid
		if c.constants != nil && c.constants.blockNum >= newHeight {
			c.constants = nil
		}

		// Clear any temporary mining entries
		c.tempConstantsMap = make(map[MiningWorkID]*MiningConstantsCache)
		c.tempEntriesMap = make(map[MiningWorkID]*MiningConfigsCache)

		// Remove all entries that are newer than or equal to the new height
		// as they might belong to the old chain
		for addr, entry := range c.entries {
			if entry.blockNum >= newHeight {
				delete(c.entries, addr)
			}
		}

		// If this is a deep reorg (more than 128 blocks), clear the entire cache
		// as a safety measure
		if c.headHeight > newHeight && c.headHeight-newHeight > 128 {
			c.constants = nil
			c.entries = make(map[common.Address]*ConfigEntry)
			log.Debug("FeeMarket cache cleared due to deep reorg", "oldHeight", c.headHeight, "newHeight", newHeight)
		}
	}

	// Update head information
	c.head = newHead
	c.parentHash = event.Block.ParentHash
	c.headHeight = newHeight
}

func (c *FeeMarketCache) Close() error {
	close(c.closeCh)
	c.chainSub.Unsubscribe()
	return nil
}

// SetConstants sets the constants in the cache
func (c *FeeMarketCache) SetConstants(constants types.FeeMarketConstants, blockNum uint64, blockHash common.Hash, workID *MiningWorkID) {
	c.lock.Lock()
	defer c.lock.Unlock()
	entry := &ConstantsEntry{
		constants: constants,
		CacheMetadata: CacheMetadata{
			blockNum:  blockNum,
			blockHash: blockHash,
			modified:  time.Now(),
		},
	}

	// If we're mining and have a workID, write to specific temp entries
	if workID != nil {
		if c.tempConstantsMap[*workID] == nil {
			// TODO: this should never happen after release (bugs free), what if we remove this check?
			c.tempConstantsMap[*workID] = &MiningConstantsCache{
				entry:  entry,
				workID: *workID,
			}
		} else {
			c.tempConstantsMap[*workID].entry = entry
		}
		return
	}

	// Otherwise, write to main cache
	c.constants = entry
}

// GetConstants gets the constants from the cache
func (c *FeeMarketCache) GetConstants(workID *MiningWorkID) *types.FeeMarketConstants {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// First check specific temp entries if we're mining
	if workID != nil {
		if tempConstants, exists := c.tempConstantsMap[*workID]; exists && tempConstants.entry != nil {
			return &tempConstants.entry.constants
		}
	}

	// Then check main cache
	if c.constants != nil && (c.constants.blockNum <= c.headHeight || (c.constants.blockNum == c.headHeight && c.constants.blockHash == c.head)) {
		return &c.constants.constants
	}
	return nil
}

// InvalidateConstants invalidates the constants in the cache
func (c *FeeMarketCache) InvalidateConstants(workID *MiningWorkID) {
	c.lock.Lock()
	if workID != nil {
		delete(c.tempConstantsMap, *workID)
	} else {
		c.constants = nil
	}
	c.lock.Unlock()
	log.Debug("FeeMarket constants invalidated", "forMiningWork", workID != nil)
}

// SetConfig sets a fee market config in the cache
func (c *FeeMarketCache) SetConfig(addr common.Address, config types.FeeMarketConfig, blockNum uint64, blockHash common.Hash, workID *MiningWorkID) {
	c.lock.Lock()
	defer c.lock.Unlock()

	entry := &ConfigEntry{
		config: config,
		CacheMetadata: CacheMetadata{
			blockNum:  blockNum,
			blockHash: blockHash,
			modified:  time.Now(),
		},
	}

	// If we're mining and have a workID, write to specific temp entries
	if workID != nil {
		if tempCache, exists := c.tempEntriesMap[*workID]; exists {
			tempCache.entries[addr] = entry
			return
		}
	}

	// Otherwise, write to main cache
	c.entries[addr] = entry
}

// GetConfig gets a fee market config from the cache
func (c *FeeMarketCache) GetConfig(addr common.Address, workID *MiningWorkID) (types.FeeMarketConfig, bool) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// First check specific temp entries if we're mining
	if workID != nil {
		if tempCache, exists := c.tempEntriesMap[*workID]; exists {
			if entry, exists := tempCache.entries[addr]; exists {
				return entry.config, true
			}
		}
	}

	// Then check main cache
	entry, exists := c.entries[addr]
	if exists && (entry.blockNum <= c.headHeight || (entry.blockNum == c.headHeight && entry.blockHash == c.head)) {
		return entry.config, true
	}
	return types.FeeMarketConfig{}, false
}

// InvalidateConfig invalidates a fee market config in the cache
func (c *FeeMarketCache) InvalidateConfig(addr common.Address, workID *MiningWorkID) {
	c.lock.Lock()
	if workID != nil {
		if tempCache, exists := c.tempEntriesMap[*workID]; exists {
			delete(tempCache.entries, addr)
		}
	} else {
		delete(c.entries, addr)
	}
	c.lock.Unlock()
	log.Debug("FeeMarket config invalidated", "address", addr, "forMiningWork", workID != nil)
}

// removeStaleEntries removes stale entries from the cache
func (c *FeeMarketCache) removeStaleEntries() {
	ticker := time.NewTicker(cacheCleanupInterval)
	for range ticker.C {
		c.lock.Lock()
		now := time.Now()
		threshold := c.headHeight
		if threshold > cacheEntryBlockThreshold {
			threshold -= cacheEntryBlockThreshold
		}
		for addr, entry := range c.entries {
			// Remove stale entries and only keep the last 256 blocks to prevent memory bloat
			if now.Sub(entry.modified) > cacheEntryExpiration || entry.blockNum < threshold {
				delete(c.entries, addr)
			}
		}
		c.lock.Unlock()
	}
}

// BeginMining starts a new mining work, supports multiple works
func (c *FeeMarketCache) BeginMining(parent common.Hash, timestamp, attemptNum uint64) MiningWorkID {
	c.lock.Lock()
	defer c.lock.Unlock()

	workID := MiningWorkID{
		ParentHash: parent,
		Timestamp:  timestamp,
		AttemptNum: attemptNum,
	}

	c.tempConstantsMap[workID] = &MiningConstantsCache{
		entry:  nil,
		workID: workID,
	}

	c.tempEntriesMap[workID] = &MiningConfigsCache{
		entries: make(map[common.Address]*ConfigEntry),
		workID:  workID,
	}

	return workID
}

// CommitMining commits a specific work to the main cache
func (c *FeeMarketCache) CommitMining(workID MiningWorkID) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Commit constants if they exist
	if tempConstants, exists := c.tempConstantsMap[workID]; exists && tempConstants.entry != nil {
		c.constants = tempConstants.entry
	}

	// Commit only the winning work's entries
	tempCache, exists := c.tempEntriesMap[workID]
	if exists && tempCache.workID.ParentHash == c.head {
		for addr, entry := range tempCache.entries {
			c.entries[addr] = entry
		}
	}

	// Cleanup all temp caches
	c.tempConstantsMap = make(map[MiningWorkID]*MiningConstantsCache)
	c.tempEntriesMap = make(map[MiningWorkID]*MiningConfigsCache)
}

// AbortMining cleans up all temp caches
func (c *FeeMarketCache) AbortMining() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.tempConstantsMap = make(map[MiningWorkID]*MiningConstantsCache)
	c.tempEntriesMap = make(map[MiningWorkID]*MiningConfigsCache)
}
