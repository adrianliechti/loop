package forward

import (
	"fmt"
	"net"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func pod(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
}

func TestStoreHandlers(t *testing.T) {
	store := NewStore[corev1.Pod]()
	handlers := storeHandlers[corev1.Pod, *corev1.Pod](store)

	handlers.OnAdd(pod("default", "a"), false)
	handlers.OnAdd(pod("other", "a"), false)

	if got := len(store.List()); got != 2 {
		t.Fatalf("after two adds: got %d items, want 2", got)
	}

	// Same namespace/name replaces rather than duplicates.
	handlers.OnUpdate(pod("default", "a"), pod("default", "a"))

	if got := len(store.List()); got != 2 {
		t.Fatalf("after update: got %d items, want 2", got)
	}

	handlers.OnDelete(pod("default", "a"))

	items := store.List()

	if len(items) != 1 {
		t.Fatalf("after delete: got %d items, want 1", len(items))
	}

	if items[0].Namespace != "other" {
		t.Fatalf("delete removed the wrong item: %s/%s", items[0].Namespace, items[0].Name)
	}
}

// A relist after a dropped watch delivers deletions the informer never observed
// as a tombstone; handling it must not panic and must still evict the object.
func TestStoreHandlersDeleteTombstone(t *testing.T) {
	store := NewStore[corev1.Pod]()
	handlers := storeHandlers[corev1.Pod, *corev1.Pod](store)

	handlers.OnAdd(pod("default", "a"), false)

	handlers.OnDelete(cache.DeletedFinalStateUnknown{
		Key: "default/a",
		Obj: pod("default", "a"),
	})

	if got := len(store.List()); got != 0 {
		t.Fatalf("tombstone delete: got %d items, want 0", got)
	}
}

// A tombstone can carry a nil object when the last known state was lost.
func TestStoreHandlersDeleteEmptyTombstone(t *testing.T) {
	store := NewStore[corev1.Pod]()
	handlers := storeHandlers[corev1.Pod, *corev1.Pod](store)

	handlers.OnAdd(pod("default", "a"), false)

	handlers.OnDelete(cache.DeletedFinalStateUnknown{Key: "default/a"})

	if got := len(store.List()); got != 1 {
		t.Fatalf("empty tombstone: got %d items, want the entry kept", got)
	}
}

func TestMapAddress(t *testing.T) {
	a := MapAddress(0, "10.0.0.1")

	if a != MapAddress(0, "10.0.0.1") {
		t.Fatal("MapAddress is not deterministic")
	}

	if a == MapAddress(1, "10.0.0.1") {
		t.Fatal("different groups mapped to the same address")
	}

	if a == MapAddress(0, "10.0.0.2") {
		t.Fatal("different cluster addresses mapped to the same address")
	}

	ip := net.ParseIP(a)

	if ip == nil {
		t.Fatalf("MapAddress returned %q, which is not an IP", a)
	}

	if !ip.IsLoopback() {
		t.Fatalf("MapAddress returned %q, which is not a loopback address", a)
	}
}

// The group lives in one bit, not a whole octet. Groups must still never
// overlap, or a catapult tunnel could take a gateway tunnel's alias.
func TestMapAddressGroupsDisjoint(t *testing.T) {
	seen := make(map[string]byte)

	for i := range 500 {
		address := fmt.Sprintf("10.0.%d.%d", i/256, i%256)

		for _, group := range []byte{0, 1} {
			mapped := MapAddress(group, address)

			if other, ok := seen[mapped]; ok && other != group {
				t.Fatalf("groups %d and %d both produced %s", other, group, mapped)
			}

			seen[mapped] = group
		}
	}
}

// Reserving a whole octet for the group left only 16 bits of digest, which
// collides well below the number of services a cluster-wide connect maps.
func TestMapAddressCollisionMargin(t *testing.T) {
	seen := make(map[string]string)

	for i := range 1500 {
		address := fmt.Sprintf("10.%d.%d.%d", i/65536, (i/256)%256, i%256)
		mapped := MapAddress(0, address)

		if previous, ok := seen[mapped]; ok {
			t.Fatalf("%s and %s both mapped to %s", previous, address, mapped)
		}

		seen[mapped] = address
	}
}
