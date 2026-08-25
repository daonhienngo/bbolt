package bbolt

import (
	"sort"
	"unsafe"
)

type FreelistType string

const (
	FreelistArrayType   FreelistType = "array"
	FreelistMapType     FreelistType = "hashmap"
	DefaultFreelistType FreelistType = FreelistArrayType
)

type freelist struct {
	freelistType FreelistType
	ids          []pgid
	pending      map[txid][]pgid
}

func newFreelist(t FreelistType) *freelist {
	if t == "" {
		t = DefaultFreelistType
	}
	return &freelist{
		freelistType: t,
		pending:      make(map[txid][]pgid),
	}
}

func (f *freelist) free(txid txid, p *page) {
	if p.id <= 1 {
		panic("cannot free meta page")
	}
	for id := p.id; id <= p.id+pgid(p.overflow); id++ {
		f.pending[txid] = append(f.pending[txid], id)
	}
}

func (f *freelist) release(txid txid) {
	m := f.pending[txid]
	if len(m) == 0 {
		return
	}
	f.ids = append(f.ids, m...)
	delete(f.pending, txid)
	sort.Slice(f.ids, func(i, j int) bool { return f.ids[i] < f.ids[j] })
}

func (f *freelist) read(p *page) {
	if (p.flags & freelistPageFlag) == 0 {
		return
	}
	count := int(p.count)
	if count == 0xFFFF {
		// Overflow count
		overflowPtr := (*pgid)(unsafe.Pointer(&p.ptr))
		count = int(*overflowPtr)
		ids := (*[1 << 28]pgid)(unsafe.Pointer(uintptr(unsafe.Pointer(&p.ptr)) + unsafe.Sizeof(pgid(0))))[:count:count]
		f.ids = append(f.ids[:0], ids...)
	} else if count > 0 {
		ids := (*[1 << 28]pgid)(unsafe.Pointer(&p.ptr))[:count:count]
		f.ids = append(f.ids[:0], ids...)
	} else {
		f.ids = f.ids[:0]
	}
	sort.Slice(f.ids, func(i, j int) bool { return f.ids[i] < f.ids[j] })
}

func (f *freelist) write(p *page) error {
	p.flags |= freelistPageFlag
	count := len(f.ids)
	if count == 0 {
		p.count = 0
		return nil
	}
	if count < 0xFFFF {
		p.count = uint16(count)
		dst := (*[1 << 28]pgid)(unsafe.Pointer(&p.ptr))[:count:count]
		copy(dst, f.ids)
	} else {
		p.count = 0xFFFF
		overflowPtr := (*pgid)(unsafe.Pointer(&p.ptr))
		*overflowPtr = pgid(count)
		dst := (*[1 << 28]pgid)(unsafe.Pointer(uintptr(unsafe.Pointer(&p.ptr)) + unsafe.Sizeof(pgid(0))))[:count:count]
		copy(dst, f.ids)
	}
	return nil
}

func (f *freelist) reload(p *page) {
	f.read(p)
}
