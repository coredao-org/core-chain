// Copyright 2020 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package state

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/metrics"
)

const (
	abortChanSize                 = 64
	concurrentChanSize            = 10
	parallelTriePrefetchThreshold = 10
	parallelTriePrefetchCapacity  = 20
)

var (
	// triePrefetchMetricsPrefix is the prefix under which to publish the metrics.
	triePrefetchMetricsPrefix = "trie/prefetch/"

	// errTerminated is returned if a fetcher is attempted to be operated after it
	// has already terminated.
	errTerminated = errors.New("fetcher is already terminated")
)

type prefetchMsg struct {
	owner common.Hash
	root  common.Hash
	addr  common.Address
	keys  [][]byte
}

// triePrefetcher is an active prefetcher, which receives accounts or storage
// items and does trie-loading of them. The goal is to get as much useful content
// into the caches as possible.
//
// Note, the prefetcher's API is not thread safe.
type triePrefetcher struct {
	db         Database               // Database to fetch trie nodes through
	root       common.Hash            // Root hash of the account trie for metrics
	rootParent common.Hash            // Root has of the account trie from block before the prvious one, designed for pipecommit mode
	fetches    map[string]Trie        // Partially or fully fetched tries. Only populated for inactive copies.
	fetchers   map[string]*subfetcher // Subfetchers for each trie

	term              chan struct{}    // Channel to signal interruption
	abortChan         chan *subfetcher // to abort a single subfetcher and its children
	closed            int32
	closeMainChan     chan bool // it is to inform the mainLoop, passing async flag to mainLoop
	closeMainDoneChan chan struct{}
	fetchersMutex     sync.RWMutex
	prefetchChan      chan *prefetchMsg // no need to wait for return

	deliveryMissMeter metrics.Meter
	accountLoadMeter  metrics.Meter
	accountDupMeter   metrics.Meter
	accountWasteMeter metrics.Meter
	storageLoadMeter  metrics.Meter
	storageDupMeter   metrics.Meter
	storageWasteMeter metrics.Meter

	accountStaleLoadMeter  metrics.Meter
	accountStaleDupMeter   metrics.Meter
	accountStaleSkipMeter  metrics.Meter
	accountStaleWasteMeter metrics.Meter
}

// newTriePrefetcher
func newTriePrefetcher(db Database, root, rootParent common.Hash, namespace string) *triePrefetcher {
	prefix := triePrefetchMetricsPrefix + namespace
	p := &triePrefetcher{
		db:         db,
		root:       root,
		rootParent: rootParent,
		fetchers:   make(map[string]*subfetcher), // Active prefetchers use the fetchers map

		term:              make(chan struct{}),
		abortChan:         make(chan *subfetcher, abortChanSize),
		closeMainChan:     make(chan bool),
		closeMainDoneChan: make(chan struct{}),
		prefetchChan:      make(chan *prefetchMsg, concurrentChanSize),

		deliveryMissMeter: metrics.GetOrRegisterMeter(prefix+"/deliverymiss", nil),
		accountLoadMeter:  metrics.GetOrRegisterMeter(prefix+"/account/load", nil),
		accountDupMeter:   metrics.GetOrRegisterMeter(prefix+"/account/dup", nil),
		accountWasteMeter: metrics.GetOrRegisterMeter(prefix+"/account/waste", nil),
		storageLoadMeter:  metrics.GetOrRegisterMeter(prefix+"/storage/load", nil),
		storageDupMeter:   metrics.GetOrRegisterMeter(prefix+"/storage/dup", nil),
		storageWasteMeter: metrics.GetOrRegisterMeter(prefix+"/storage/waste", nil),

		accountStaleLoadMeter:  metrics.GetOrRegisterMeter(prefix+"/accountst/load", nil),
		accountStaleDupMeter:   metrics.GetOrRegisterMeter(prefix+"/accountst/dup", nil),
		accountStaleSkipMeter:  metrics.GetOrRegisterMeter(prefix+"/accountst/skip", nil),
		accountStaleWasteMeter: metrics.GetOrRegisterMeter(prefix+"/accountst/waste", nil),
	}
	go p.mainLoop()
	return p
}

// the subfetcher's lifecycle will only be updated in this loop,
// include: subfetcher's creation & abort, child subfetcher's creation & abort.
// since the mainLoop will handle all the requests, each message handle should be lightweight
func (p *triePrefetcher) mainLoop() {
	for {
		select {
		case pMsg := <-p.prefetchChan:
			id := p.trieID(pMsg.owner, pMsg.root)
			fetcher := p.fetchers[id]
			if fetcher == nil {
				fetcher = newSubfetcher(p.db, p.root, pMsg.owner, pMsg.root, pMsg.addr)
				p.fetchersMutex.Lock()
				p.fetchers[id] = fetcher
				p.fetchersMutex.Unlock()
			}
			select {
			case <-fetcher.stop:
			default:
				fetcher.schedule(pMsg.keys)
				// no need to run parallel trie prefetch if threshold is not reached.
				if atomic.LoadUint32(&fetcher.pendingSize) > parallelTriePrefetchThreshold {
					fetcher.scheduleParallel(pMsg.keys)
				}
			}

		case fetcher := <-p.abortChan:
			fetcher.terminate(false)
			for _, child := range fetcher.paraChildren {
				child.terminate(false)
			}

		case async := <-p.closeMainChan:
			for _, fetcher := range p.fetchers {
				fetcher.terminate(async) // safe to do multiple times
				for _, child := range fetcher.paraChildren {
					child.terminate(async)
				}
			}

			close(p.term)
			close(p.closeMainDoneChan)
			p.fetchersMutex.Lock()
			p.fetchers = nil
			p.fetchersMutex.Unlock()
			return
		}
	}
}

// close iterates over all the subfetchers, aborts any that were left spinning
// and reports the stats to the metrics subsystem.
func (p *triePrefetcher) terminate(async bool) {
	// Short circuit if the fetcher is already closed
	select {
	case <-p.term:
		return
	default:
	}
	if atomic.CompareAndSwapInt32(&p.closed, 0, 1) {
		p.closeMainChan <- async
		<-p.closeMainDoneChan // wait until all subfetcher are stopped
	}
}

// report aggregates the pre-fetching and usage metrics and reports them.
func (p *triePrefetcher) report() {
	if !metrics.Enabled {
		return
	}
	// make sure all subfetchers and child subfetchers are stopped
	for _, fetcher := range p.fetchers {
		fetcher.wait() // ensure the fetcher's idle before poking in its internals

		if fetcher.root == p.root {
			p.accountLoadMeter.Mark(int64(len(fetcher.seen)))
			p.accountDupMeter.Mark(int64(fetcher.dups))
			for _, key := range fetcher.used {
				delete(fetcher.seen, string(key))
			}
			p.accountWasteMeter.Mark(int64(len(fetcher.seen)))
		} else {
			p.storageLoadMeter.Mark(int64(len(fetcher.seen)))
			p.storageDupMeter.Mark(int64(fetcher.dups))
			for _, key := range fetcher.used {
				delete(fetcher.seen, string(key))
			}
			p.storageWasteMeter.Mark(int64(len(fetcher.seen)))
		}
	}
}

// prefetch schedules a batch of trie items to prefetch.
func (p *triePrefetcher) prefetch(owner common.Hash, root common.Hash, addr common.Address, keys [][]byte) error {
	// Ensure the subfetcher is still alive
	select {
	case <-p.term:
		return errTerminated
	default:
	}

	select {
	case <-p.term:
		return errTerminated
	case p.prefetchChan <- &prefetchMsg{owner, root, addr, keys}:
	}
	return nil
}

// trie returns the trie matching the root hash, or nil if the prefetcher doesn't
// have it.
func (p *triePrefetcher) trie(owner common.Hash, root common.Hash) (Trie, error) {
	// Bail if no trie was prefetched for this root
	id := p.trieID(owner, root)

	// use lock instead of request to mainLoop by chan to get the fetcher for performance concern.
	p.fetchersMutex.RLock()
	fetcher := p.fetchers[id]
	p.fetchersMutex.RUnlock()
	if fetcher == nil {
		log.Error("Prefetcher missed to load trie", "owner", owner, "root", root)
		p.deliveryMissMeter.Mark(1)
		return nil, nil
	}
	// Subfetcher exists, retrieve its trie
	return fetcher.peek(), nil
}

// used marks a batch of state items used to allow creating statistics as to
// how useful or wasteful the fetcher is.
func (p *triePrefetcher) used(owner common.Hash, root common.Hash, used [][]byte) {
	// If the prefetcher is an inactive one, bail out
	if p.fetches != nil {
		return
	}
	select {
	case <-p.term:
	default:
		p.fetchersMutex.RLock()
		id := p.trieID(owner, root)
		if fetcher := p.fetchers[id]; fetcher != nil {
			fetcher.lock.Lock()
			fetcher.used = used
			fetcher.lock.Unlock()
		}
		p.fetchersMutex.RUnlock()
	}
}

// trieID returns an unique trie identifier consists the trie owner and root hash.
func (p *triePrefetcher) trieID(owner common.Hash, root common.Hash) string {
	trieID := make([]byte, common.HashLength*2)
	copy(trieID, owner.Bytes())
	copy(trieID[common.HashLength:], root.Bytes())
	return string(trieID)
}

// subfetcher is a trie fetcher goroutine responsible for pulling entries for a
// single trie. It is spawned when a new root is encountered and lives until the
// main prefetcher is paused and either all requested items are processed or if
// the trie being worked on is retrieved from the prefetcher.
type subfetcher struct {
	db    Database       // Database to load trie nodes through
	state common.Hash    // Root hash of the state to prefetch
	owner common.Hash    // Owner of the trie, usually account hash
	root  common.Hash    // Root hash of the trie to prefetch
	addr  common.Address // Address of the account that the trie belongs to
	trie  Trie           // Trie being populated with nodes

	tasks [][]byte   // Items queued up for retrieval
	lock  sync.Mutex // Lock protecting the task queue

	wake chan struct{} // Wake channel if a new task is scheduled
	stop chan struct{} // Channel to interrupt processing
	term chan struct{} // Channel to signal interruption

	seen map[string]struct{} // Tracks the entries already loaded
	dups int                 // Number of duplicate preload tasks
	used [][]byte            // Tracks the entries used in the end

	pendingSize  uint32
	paraChildren []*subfetcher // Parallel trie prefetch for address of massive change
}

// newSubfetcher creates a goroutine to prefetch state items belonging to a
// particular root hash.
func newSubfetcher(db Database, state common.Hash, owner common.Hash, root common.Hash, addr common.Address) *subfetcher {
	sf := &subfetcher{
		db:    db,
		state: state,
		owner: owner,
		root:  root,
		addr:  addr,
		wake:  make(chan struct{}, 1),
		stop:  make(chan struct{}),
		term:  make(chan struct{}),
		seen:  make(map[string]struct{}),
	}
	go sf.loop()
	return sf
}

// schedule adds a batch of trie keys to the queue to prefetch.
func (sf *subfetcher) schedule(keys [][]byte) error {
	// Ensure the subfetcher is still alive
	select {
	case <-sf.term:
		return errTerminated
	default:
	}
	atomic.AddUint32(&sf.pendingSize, uint32(len(keys)))
	// Append the tasks to the current queue
	sf.lock.Lock()
	sf.tasks = append(sf.tasks, keys...)
	sf.lock.Unlock()
	// Notify the background thread to execute scheduled tasks
	select {
	case sf.wake <- struct{}{}:
		// Wake signal sent
	default:
		// Wake signal not sent as a previous is already queued
	}
	return nil
}

// wait blocks until the subfetcher terminates. This method is used to block on
// an async termination before accessing internal fields from the fetcher.
func (sf *subfetcher) wait() {
	<-sf.term
	for _, child := range sf.paraChildren {
		<-child.term
	}
}

func (sf *subfetcher) scheduleParallel(keys [][]byte) {
	var keyIndex uint32 = 0
	childrenNum := len(sf.paraChildren)
	if childrenNum > 0 {
		// To feed the children first, if they are hungry.
		// A child can handle keys with capacity of parallelTriePrefetchCapacity.
		childIndex := len(keys) % childrenNum // randomly select the start child to avoid always feed the first one
		for i := 0; i < childrenNum; i++ {
			child := sf.paraChildren[childIndex]
			childIndex = (childIndex + 1) % childrenNum
			if atomic.LoadUint32(&child.pendingSize) >= parallelTriePrefetchCapacity {
				// the child is already full, skip it
				continue
			}
			feedNum := parallelTriePrefetchCapacity - atomic.LoadUint32(&child.pendingSize)
			if keyIndex+feedNum >= uint32(len(keys)) {
				// the new arrived keys are all consumed by children.
				child.schedule(keys[keyIndex:])
				return
			}
			child.schedule(keys[keyIndex : keyIndex+feedNum])
			keyIndex += feedNum
		}
	}
	// Children did not consume all the keys, to create new subfetch to handle left keys.
	keysLeft := keys[keyIndex:]
	keysLeftSize := len(keysLeft)
	for i := 0; i*parallelTriePrefetchCapacity < keysLeftSize; i++ {
		child := newSubfetcher(sf.db, sf.state, sf.owner, sf.root, sf.addr)
		sf.paraChildren = append(sf.paraChildren, child)
		endIndex := (i + 1) * parallelTriePrefetchCapacity
		if endIndex >= keysLeftSize {
			child.schedule(keysLeft[i*parallelTriePrefetchCapacity:])
			return
		}
		child.schedule(keysLeft[i*parallelTriePrefetchCapacity : endIndex])
	}
}

// peek tries to retrieve a deep copy of the fetcher's trie in whatever form it
// is currently.
func (sf *subfetcher) peek() Trie {
	// Block until the fertcher terminates, then retrieve the trie
	sf.wait()
	return sf.trie
}

// terminate requests the subfetcher to stop accepting new tasks and spin down
// as soon as everything is loaded. Depending on the async parameter, the method
// will either block until all disk loads finish or return immediately.
func (sf *subfetcher) terminate(async bool) {
	select {
	case <-sf.stop:
	default:
		close(sf.stop)
	}
}

// loop loads newly-scheduled trie tasks as they are received and loads them, stopping
// when requested.
func (sf *subfetcher) loop() {
	// No matter how the loop stops, signal anyone waiting that it's terminated
	defer close(sf.term)

	// Start by opening the trie and stop processing if it fails
	var trie Trie
	var err error
	if sf.owner == (common.Hash{}) {
		trie, err = sf.db.OpenTrie(sf.root)
	} else {
		trie, err = sf.db.OpenStorageTrie(sf.state, sf.addr, sf.root, nil)
	}
	if err != nil {
		log.Debug("Trie prefetcher failed opening trie", "root", sf.root, "err", err)
		return
	}
	sf.trie = trie

	// Trie opened successfully, keep prefetching items
	for {
		select {
		case <-sf.wake:
			// Execute all remaining tasks in single run
			if sf.trie == nil {
				if sf.owner == (common.Hash{}) {
					sf.trie, err = sf.db.OpenTrie(sf.root)
				} else {
					// address is useless
					sf.trie, err = sf.db.OpenStorageTrie(sf.state, sf.addr, sf.root, nil)
				}
				if err != nil {
					continue
				}
			}

			sf.lock.Lock()
			tasks := sf.tasks
			sf.tasks = nil
			sf.lock.Unlock()

			// Prefetch any tasks until the loop is interrupted
			for _, task := range tasks {
				if _, ok := sf.seen[string(task)]; ok {
					sf.dups++
				} else {
					if len(task) == common.AddressLength {
						sf.trie.GetAccount(common.BytesToAddress(task))
					} else {
						sf.trie.GetStorage(sf.addr, task)
					}
					sf.seen[string(task)] = struct{}{}
				}
				atomic.AddUint32(&sf.pendingSize, ^uint32(0)) // decrease
			}

		case <-sf.stop:
			// Termination is requested, abort if no more tasks are pending. If
			// there are some, exhaust them first.
			sf.lock.Lock()
			done := sf.tasks == nil
			sf.lock.Unlock()

			if done {
				return
			}
			// Some tasks are pending, loop and pick them up (that wake branch
			// will be selected eventually, whilst stop remains closed to this
			// branch will also run afterwards).
		}
	}
}
