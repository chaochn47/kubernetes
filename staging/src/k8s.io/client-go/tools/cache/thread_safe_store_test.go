/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cache

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestThreadSafeStoreDeleteRemovesEmptySetsFromIndex(t *testing.T) {
	testIndexer := "testIndexer"

	indexers := Indexers{
		testIndexer: func(obj interface{}) (strings []string, e error) {
			indexes := []string{obj.(string)}
			return indexes, nil
		},
	}

	indices := Indices{}
	store := NewThreadSafeStore(indexers, indices).(*threadSafeMap)

	testKey := "testKey"

	store.Add(testKey, testKey)

	// Assumption check, there should be a set for the `testKey` with one element in the added index
	set := store.index.indices[testIndexer][testKey]

	if len(set) != 1 {
		t.Errorf("Initial assumption of index backing string set having 1 element failed. Actual elements: %d", len(set))
		return
	}

	store.Delete(testKey)
	set, present := store.index.indices[testIndexer][testKey]

	if present {
		t.Errorf("Index backing string set not deleted from index. Set length: %d", len(set))
	}
}

func TestThreadSafeStoreAddKeepsNonEmptySetPostDeleteFromIndex(t *testing.T) {
	testIndexer := "testIndexer"
	testIndex := "testIndex"

	indexers := Indexers{
		testIndexer: func(obj interface{}) (strings []string, e error) {
			indexes := []string{testIndex}
			return indexes, nil
		},
	}

	indices := Indices{}
	store := NewThreadSafeStore(indexers, indices).(*threadSafeMap)

	store.Add("retain", "retain")
	store.Add("delete", "delete")

	// Assumption check, there should be a set for the `testIndex` with two elements
	set := store.index.indices[testIndexer][testIndex]

	if len(set) != 2 {
		t.Errorf("Initial assumption of index backing string set having 2 elements failed. Actual elements: %d", len(set))
		return
	}

	store.Delete("delete")
	set, present := store.index.indices[testIndexer][testIndex]

	if !present {
		t.Errorf("Index backing string set erroneously deleted from index.")
		return
	}

	if len(set) != 1 {
		t.Errorf("Index backing string set has incorrect length, expect 1. Set length: %d", len(set))
	}
}

func TestThreadSafeStoreIndexingFunctionsWithMultipleValues(t *testing.T) {
	testIndexer := "testIndexer"

	indexers := Indexers{
		testIndexer: func(obj interface{}) ([]string, error) {
			return strings.Split(obj.(string), ","), nil
		},
	}

	indices := Indices{}
	store := NewThreadSafeStore(indexers, indices).(*threadSafeMap)

	store.Add("key1", "foo")
	store.Add("key2", "bar")

	assert := assert.New(t)

	compare := func(key string, expected []string) error {
		values := store.index.indices[testIndexer][key].List()
		if cmp.Equal(values, expected) {
			return nil
		}
		return fmt.Errorf("unexpected index for key %s, diff=%s", key, cmp.Diff(values, expected))
	}

	assert.NoError(compare("foo", []string{"key1"}))
	assert.NoError(compare("bar", []string{"key2"}))

	store.Update("key2", "foo,bar")

	assert.NoError(compare("foo", []string{"key1", "key2"}))
	assert.NoError(compare("bar", []string{"key2"}))

	store.Update("key1", "foo,bar")

	assert.NoError(compare("foo", []string{"key1", "key2"}))
	assert.NoError(compare("bar", []string{"key1", "key2"}))

	store.Add("key3", "foo,bar,baz")

	assert.NoError(compare("foo", []string{"key1", "key2", "key3"}))
	assert.NoError(compare("bar", []string{"key1", "key2", "key3"}))
	assert.NoError(compare("baz", []string{"key3"}))

	store.Update("key1", "foo")

	assert.NoError(compare("foo", []string{"key1", "key2", "key3"}))
	assert.NoError(compare("bar", []string{"key2", "key3"}))
	assert.NoError(compare("baz", []string{"key3"}))

	store.Update("key2", "bar")

	assert.NoError(compare("foo", []string{"key1", "key3"}))
	assert.NoError(compare("bar", []string{"key2", "key3"}))
	assert.NoError(compare("baz", []string{"key3"}))

	store.Delete("key1")

	assert.NoError(compare("foo", []string{"key3"}))
	assert.NoError(compare("bar", []string{"key2", "key3"}))
	assert.NoError(compare("baz", []string{"key3"}))

	store.Delete("key3")

	assert.NoError(compare("foo", []string{}))
	assert.NoError(compare("bar", []string{"key2"}))
	assert.NoError(compare("baz", []string{}))
}

func BenchmarkIndexer(b *testing.B) {
	testIndexer := "testIndexer"

	indexers := Indexers{
		testIndexer: func(obj interface{}) (strings []string, e error) {
			indexes := []string{obj.(string)}
			return indexes, nil
		},
	}

	indices := Indices{}
	store := NewThreadSafeStore(indexers, indices).(*threadSafeMap)

	// The following benchmark imitates what is happening in indexes
	// used in storage layer, where indexing is mostly static (e.g.
	// indexing objects by their (namespace, name)).
	// The 5000 number imitates indexing nodes in 5000-node cluster.
	objectCount := 5000
	objects := make([]string, 0, 5000)
	for i := 0; i < objectCount; i++ {
		objects = append(objects, fmt.Sprintf("object-number-%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Update(objects[i%objectCount], objects[i%objectCount])
	}
}

func BenchmarkIndexerWithConcurrentReadWrite(b *testing.B) {
	testIndexer := "testIndexer"

	indexers := Indexers{
		testIndexer: func(obj interface{}) (strings []string, e error) {
			indexes := []string{obj.(string)}
			return indexes, nil
		},
	}

	indices := Indices{}
	store := NewThreadSafeStore(indexers, indices).(*threadSafeMap)
	//store := threadSafeMap{items: map[string]interface{}{}}

	// Pre-populate store with 70k objects
	for i := 0; i < 70000; i++ {
		obj := fmt.Sprintf("initial-object-%d", i)
		store.Update(obj, obj)
	}

	// Create a WaitGroup to synchronize goroutines
	var wg sync.WaitGroup

	// Create a channel to signal completion
	done := make(chan struct{})

	// Add counters for read operations
	readOps := uint64(0)

	b.ResetTimer()

	// Start 5 reader goroutines
	for r := 0; r < 5; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					store.List()                  // Continuous reading
					atomic.AddUint64(&readOps, 1) // Count read operations
				}
			}
		}()
	}

	startTime := time.Now()
	writeOps := 0

	// Benchmark the writer
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			obj := fmt.Sprintf("new-object-%d", i%800000)
			store.Update(obj, obj)
			writeOps++
			i++
		}
	})

	elapsed := time.Since(startTime)

	// Signal readers to stop and wait for completion
	close(done)
	wg.Wait()

	// Report throughput metrics
	totalReads := atomic.LoadUint64(&readOps)
	readThroughput := float64(totalReads) / elapsed.Seconds()
	writeThroughput := float64(writeOps) / elapsed.Seconds()

	b.ReportMetric(readThroughput, "reads/sec")
	b.ReportMetric(writeThroughput, "writes/sec")
}
