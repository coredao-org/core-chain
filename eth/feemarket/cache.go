package feemarket

import (
	"errors"
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
	blockNum uint64
	modified time.Time
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
	constants        *ConstantsEntry                        // main cache for constants
	tempConstantsMap map[MiningWorkID]*MiningConstantsCache // temp constants cache for mining

	entries        map[common.Address]*ConfigEntry      // main cache for configs
	tempEntriesMap map[MiningWorkID]*MiningConfigsCache // temp configs cache for mining

	head       common.Hash // current head hash
	headHeight uint64      // current head height
	parentHash common.Hash // current head parent hash

	chainSub event.Subscription  // chain subscription
	chainCh  chan ChainHeadEvent // chain channel
	closeCh  chan struct{}       // close channel

	lock sync.RWMutex
}

func NewFeeMarketCache(reader BlockChain) (*FeeMarketCache, error) {
	if reader == nil {
		return nil, errors.New("blockchain reader is not provided")
	}

	currentHeader := reader.CurrentHeader()

	c := &FeeMarketCache{
		tempConstantsMap: make(map[MiningWorkID]*MiningConstantsCache),
		entries:          make(map[common.Address]*ConfigEntry),
		tempEntriesMap:   make(map[MiningWorkID]*MiningConfigsCache),

		head:       currentHeader.Hash(),
		headHeight: currentHeader.Number.Uint64(),
		parentHash: currentHeader.ParentHash,

		chainCh: make(chan ChainHeadEvent, ChainHeadChanSize),
		closeCh: make(chan struct{}),
	}

	// Subscribe to chain events
	c.chainSub = reader.FeeMarketSubscribeChainHeadEvent(c.chainCh)

	go c.loop()
	go c.removeStaleEntries()

	return c, nil
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
func (c *FeeMarketCache) onChainEvent(event ChainHeadEvent) {
	c.lock.Lock()
	defer func() {
		jsonReport, err := c.ReportJSON()
		if err != nil {
			log.Error("Failed to generate cache report", "err", err)
			return
		}
		log.Info("FeeMarket cache state", "cacheBlockNumber", c.headHeight, "cacheBlockHash", c.head, "cacheJson", jsonReport)
	}()

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
func (c *FeeMarketCache) SetConstants(constants types.FeeMarketConstants, blockNum uint64, workID *MiningWorkID) {
	// TODO: remove this for release
	log.Debug("FeeMarket constants set", "blockNum", blockNum)

	entry := &ConstantsEntry{
		constants: constants,
		CacheMetadata: CacheMetadata{
			blockNum: blockNum,
			modified: time.Now(),
		},
	}

	c.lock.Lock()
	defer c.lock.Unlock()

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
func (c *FeeMarketCache) GetConstants(blockNumber uint64, workID *MiningWorkID) *types.FeeMarketConstants {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// First check specific temp entries if we're mining
	if workID != nil {
		if tempConstants, exists := c.tempConstantsMap[*workID]; exists && tempConstants.entry != nil {
			return &tempConstants.entry.constants
		}
	}

	// Then check main cache
	// TODO: the blockHash == c.head will probably never happen on mining, shall we remove it?
	// We can also skip writing to cache if c.constants.blockNum == blockNumber, except if it's through BeginMining
	if c.constants != nil && (c.constants.blockNum < blockNumber || (c.constants.blockNum == blockNumber && c.constants.blockHash == c.head)) {
		return &c.constants.constants
	}
	return nil
}

// InvalidateConstants invalidates the constants in the cache
func (c *FeeMarketCache) InvalidateConstants(workID *MiningWorkID) {
	log.Debug("FeeMarket constants invalidated", "forMiningWork", workID != nil)

	c.lock.Lock()
	defer c.lock.Unlock()
	if workID != nil {
		delete(c.tempConstantsMap, *workID)
	} else {
		c.constants = nil
	}
}

// SetConfig sets a fee market config in the cache
func (c *FeeMarketCache) SetConfig(addr common.Address, config types.FeeMarketConfig, blockNum uint64, workID *MiningWorkID) {
	// TODO: remove this for release
	log.Debug("FeeMarket config set", "address", addr, "blockNum", blockNum)

	c.lock.Lock()
	defer c.lock.Unlock()

	entry := &ConfigEntry{
		config: config,
		CacheMetadata: CacheMetadata{
			blockNum: blockNum,
			modified: time.Now(),
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
func (c *FeeMarketCache) GetConfig(addr common.Address, blockNumber uint64, workID *MiningWorkID) (types.FeeMarketConfig, bool) {
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
	// TODO: the blockHash == c.head will probably never happen on mining, shall we remove it?
	// We can also skip writing to cache if c.constants.blockNum == blockNumber, except if it's through BeginMining
	if exists && (entry.blockNum < blockNumber || (entry.blockNum == blockNumber && entry.blockHash == c.head)) {
		return entry.config, true
	}
	return types.FeeMarketConfig{}, false
}

// InvalidateConfig invalidates a fee market config in the cache
func (c *FeeMarketCache) InvalidateConfig(addr common.Address, workID *MiningWorkID) {
	log.Debug("FeeMarket config invalidated", "address", addr, "forMiningWork", workID != nil)

	c.lock.Lock()
	defer c.lock.Unlock()
	if workID != nil {
		if tempCache, exists := c.tempEntriesMap[*workID]; exists {
			delete(tempCache.entries, addr)
		}
	} else {
		delete(c.entries, addr)
	}
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
				// TODO: remove this for release
				log.Debug("FeeMarket cache stale entry removed", "address", addr, "blockNum", entry.blockNum)
			}
		}
		c.lock.Unlock()
	}
}

// BeginMining begins a new mining session,
// multiple mining sessions can be active at the same time for the same block
func (c *FeeMarketCache) BeginMining(parent common.Hash, timestamp, attemptNum uint64) MiningWorkID {
	// TODO: remove this for release
	log.Debug("FeeMarket mining started", "parent", parent, "attemptNum", attemptNum, "timestamp", timestamp)

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
		if tempConstants.workID.ParentHash == c.head {
			c.constants = tempConstants.entry
		}
	}

	// Commit only the winning work's entries
	tempCache, exists := c.tempEntriesMap[workID]
	if exists && tempCache.workID.ParentHash == c.head {
		for addr, entry := range tempCache.entries {
			c.entries[addr] = entry
			// TODO: remove this for release
			log.Debug("FeeMarket cache mining entry committed", "address", addr, "blockNum", entry.blockNum, "attemptNum", tempCache.workID.AttemptNum)
		}
	}

	// Cleanup all temp caches
	c.tempConstantsMap = make(map[MiningWorkID]*MiningConstantsCache)
	c.tempEntriesMap = make(map[MiningWorkID]*MiningConfigsCache)
}

// AbortMining cleans up all temp caches
func (c *FeeMarketCache) AbortMining() {
	// TODO: remove this for release
	log.Debug("FeeMarket cache block mining aborted")

	c.lock.Lock()
	defer c.lock.Unlock()
	c.tempConstantsMap = make(map[MiningWorkID]*MiningConstantsCache)
	c.tempEntriesMap = make(map[MiningWorkID]*MiningConfigsCache)
}
