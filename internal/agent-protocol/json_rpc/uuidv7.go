package json_rpc

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type uuidv7Gen struct {
	mu      sync.Mutex
	lastMs  int64
	counter uint32
}

func (g *uuidv7Gen) Next() string {
	g.mu.Lock()
	ms := time.Now().UnixMilli()
	if ms == g.lastMs {
		g.counter++
	} else {
		g.counter = 0
		g.lastMs = ms
	}
	c := g.counter
	g.mu.Unlock()

	b := make([]byte, 16)
	binary.BigEndian.PutUint16(b[0:2], uint16(ms>>32))
	binary.BigEndian.PutUint32(b[2:6], uint32(ms&0xffffffff))

	var r [10]byte
	if _, err := rand.Read(r[:]); err != nil {
		panic(fmt.Sprintf("uuidv7: crypto/rand failed: %v", err))
	}

	b[6] = 0x70 | (r[0] & 0x0f)
	b[7] = r[1]
	b[8] = 0x80 | (r[2] & 0x3f)

	hi := uint32(c) & 0xffffff
	b[9] = byte(hi >> 16)
	b[10] = byte(hi >> 8)
	b[11] = byte(hi)

	copy(b[12:], r[3:])

	s := hex.EncodeToString(b)
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}

var defaultUUIDv7 = &uuidv7Gen{}

func NewV7() string { return defaultUUIDv7.Next() }
