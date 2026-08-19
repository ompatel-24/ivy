package session

// byteRing retains only the newest bytes up to its fixed capacity. Callers
// provide synchronization so snapshots can be coordinated with subscribers.
type byteRing struct {
	buffer []byte
	start  int
	size   int
}

func newByteRing(capacity int) *byteRing {
	return &byteRing{buffer: make([]byte, capacity)}
}

func (r *byteRing) Write(data []byte) {
	if len(data) == 0 || len(r.buffer) == 0 {
		return
	}
	if len(data) >= len(r.buffer) {
		copy(r.buffer, data[len(data)-len(r.buffer):])
		r.start = 0
		r.size = len(r.buffer)
		return
	}

	for _, value := range data {
		if r.size < len(r.buffer) {
			position := (r.start + r.size) % len(r.buffer)
			r.buffer[position] = value
			r.size++
			continue
		}
		r.buffer[r.start] = value
		r.start = (r.start + 1) % len(r.buffer)
	}
}

func (r *byteRing) Bytes() []byte {
	result := make([]byte, r.size)
	if r.size == 0 {
		return result
	}

	first := min(r.size, len(r.buffer)-r.start)
	copy(result, r.buffer[r.start:r.start+first])
	copy(result[first:], r.buffer[:r.size-first])
	return result
}

func (r *byteRing) Len() int {
	return r.size
}

func (r *byteRing) Cap() int {
	return len(r.buffer)
}
