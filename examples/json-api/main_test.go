package main

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestStoreReturnsSnapshots(t *testing.T) {
	store := NewTodoStore()
	created := store.Create("original")
	got, _ := store.Get(created.ID)
	listed := store.List()[0]
	updated, _ := store.Update(created.ID, todoInput{Title: "updated"})
	store.Update(created.ID, todoInput{Title: "latest"})
	if created.Title != "original" || got.Title != "original" || listed.Title != "original" || updated.Title != "updated" {
		t.Fatal("store leaked a mutable record")
	}
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			for range 100 {
				store.Update(created.ID, todoInput{Title: "concurrent"})
				todo, _ := store.Get(created.ID)
				if _, err := json.Marshal(todo); err != nil {
					t.Error(err)
				}
				if _, err := json.Marshal(store.List()); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
}
