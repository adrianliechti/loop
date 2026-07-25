package forward

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// object is any API type whose pointer form can be handled by an informer.
type object[T any] interface {
	*T

	metav1.Object
	runtime.Object
}

// Store is a namespace/name keyed snapshot of one resource kind, kept current
// by an informer.
type Store[T any] struct {
	mu    sync.Mutex
	items map[string]T
}

func NewStore[T any]() *Store[T] {
	return &Store[T]{
		items: make(map[string]T),
	}
}

func (s *Store[T]) List() []T {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]T, 0, len(s.items))

	for _, key := range slices.Sorted(maps.Keys(s.items)) {
		items = append(items, s.items[key])
	}

	return items
}

func (s *Store[T]) set(key string, item T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items[key] = item
}

func (s *Store[T]) delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.items, key)
}

// Watch seeds the store with items and then keeps it up to date from an
// informer that runs until ctx is done. The caller lists explicitly so the
// store is already populated on return: the informer's own initial sync is
// asynchronous and would leave the first reconcile looking at an empty store.
func Watch[T any, PT object[T]](ctx context.Context, store *Store[T], getter cache.Getter, resource, namespace string, items []T) {
	for _, item := range items {
		var p PT = &item
		store.set(key(p), item)
	}

	lw := cache.NewListWatchFromClient(getter, resource, namespace, fields.Everything())

	_, controller := cache.NewInformer(lw, PT(new(T)), 0, storeHandlers[T, PT](store))
	go controller.Run(ctx.Done())
}

func storeHandlers[T any, PT object[T]](store *Store[T]) cache.ResourceEventHandlerFuncs {
	set := func(obj any) {
		if p, ok := obj.(PT); ok {
			store.set(key(p), *p)
		}
	}

	return cache.ResourceEventHandlerFuncs{
		AddFunc: set,

		UpdateFunc: func(_, obj any) {
			set(obj)
		},

		DeleteFunc: func(obj any) {
			// A relist after a dropped watch reports deletions it never
			// observed as a tombstone wrapping the last known object, so
			// asserting straight to PT here would panic.
			if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = tombstone.Obj
			}

			if p, ok := obj.(PT); ok {
				store.delete(key(p))
			}
		},
	}
}

func key(obj metav1.Object) string {
	return fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())
}
