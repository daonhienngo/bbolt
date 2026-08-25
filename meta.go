package bbolt

import (
	"encoding/binary"
	"hash/fnv"
	"unsafe"
)

const (
	magic   uint32 = 0xED0CDAED
	Version uint32 = 2
)

type meta struct {
	magic    uint32
	version  uint32
	pageSize uint32
	flags    uint32
	root     bucketHeader
	freelist pgid
	pgid     pgid
	txid     txid
	checksum uint64
}

type bucketHeader struct {
	root     pgid
	sequence uint64
}

func (m *meta) validate() error {
	if m.magic != magic {
		return ErrInvalid
	} else if m.version != Version {
		return ErrVersionMismatch
	} else if m.checksum != 0 && m.checksum != m.sum64() {
		return ErrChecksum
	}
	return nil
}

func (m *meta) sum64() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	binary.LittleEndian.PutUint32(buf[0:4], m.magic)
	binary.LittleEndian.PutUint32(buf[4:8], m.version)
	_, _ = h.Write(buf[:8])
	binary.LittleEndian.PutUint32(buf[0:4], m.pageSize)
	binary.LittleEndian.PutUint32(buf[4:8], m.flags)
	_, _ = h.Write(buf[:8])
	binary.LittleEndian.PutUint64(buf[0:8], uint64(m.root.root))
	_, _ = h.Write(buf[:8])
	binary.LittleEndian.PutUint64(buf[0:8], m.root.sequence)
	_, _ = h.Write(buf[:8])
	binary.LittleEndian.PutUint64(buf[0:8], uint64(m.freelist))
	_, _ = h.Write(buf[:8])
	binary.LittleEndian.PutUint64(buf[0:8], uint64(m.pgid))
	_, _ = h.Write(buf[:8])
	binary.LittleEndian.PutUint64(buf[0:8], uint64(m.txid))
	_, _ = h.Write(buf[:8])
	return h.Sum64()
}

func (m *meta) write(p *page) {
	if m.root.root >= m.pgid && m.root.root != 0 {
		panic("root pgid exceeds total page count")
	}
	p.id = pgid(m.txid % 2)
	p.flags |= metaPageFlag
	m.checksum = m.sum64()
	mDest := (*meta)(unsafe.Pointer(&p.ptr))
	*mDest = *m
}
