package session

import (
	"bytes"
	"testing"
)

func TestByteRing(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		writes   [][]byte
		want     []byte
	}{
		{
			name:     "empty",
			capacity: 4,
		},
		{
			name:     "under capacity",
			capacity: 8,
			writes:   [][]byte{[]byte("ab"), []byte("cd")},
			want:     []byte("abcd"),
		},
		{
			name:     "wraps",
			capacity: 5,
			writes:   [][]byte{[]byte("abc"), []byte("defg")},
			want:     []byte("cdefg"),
		},
		{
			name:     "oversize write",
			capacity: 4,
			writes:   [][]byte{[]byte("before"), []byte("123456")},
			want:     []byte("3456"),
		},
		{
			name:     "zero capacity",
			capacity: 0,
			writes:   [][]byte{[]byte("discarded")},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ring := newByteRing(test.capacity)
			for _, data := range test.writes {
				ring.Write(data)
			}
			if got := ring.Bytes(); !bytes.Equal(got, test.want) {
				t.Fatalf("Bytes() = %q, want %q", got, test.want)
			}
			if ring.Len() != len(test.want) {
				t.Fatalf("Len() = %d, want %d", ring.Len(), len(test.want))
			}
			if ring.Cap() != test.capacity {
				t.Fatalf("Cap() = %d, want %d", ring.Cap(), test.capacity)
			}
		})
	}
}

func TestByteRingSnapshotIsIndependent(t *testing.T) {
	ring := newByteRing(8)
	ring.Write([]byte("abcd"))
	snapshot := ring.Bytes()
	snapshot[0] = 'X'
	if got := ring.Bytes(); !bytes.Equal(got, []byte("abcd")) {
		t.Fatalf("mutating snapshot changed ring: %q", got)
	}
}
