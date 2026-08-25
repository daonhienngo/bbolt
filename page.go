package bbolt

import (
	"fmt"
	"unsafe"
)

const (
	branchPageFlag   = 0x01
	leafPageFlag     = 0x02
	metaPageFlag     = 0x04
	freelistPageFlag = 0x10
)

const pageHeaderSize = int(unsafe.Offsetof(page{}.ptr))

type pgid uint64
type txid uint64

type page struct {
	id       pgid
	flags    uint16
	count    uint16
	overflow uint32
	ptr      uintptr
}

func (p *page) typ() string {
	if (p.flags & branchPageFlag) != 0 {
		return "branch"
	} else if (p.flags & leafPageFlag) != 0 {
		return "leaf"
	} else if (p.flags & metaPageFlag) != 0 {
		return "meta"
	} else if (p.flags & freelistPageFlag) != 0 {
		return "freelist"
	}
	return fmt.Sprintf("unknown<%02x>", p.flags)
}

func (p *page) meta() *meta {
	return (*meta)(unsafe.Pointer(&p.ptr))
}

func (p *page) leafPageElement(index uint16) *leafPageElement {
	n := &leafPageElement{}
	elemSize := unsafe.Sizeof(*n)
	ptr := unsafe.Pointer(uintptr(unsafe.Pointer(&p.ptr)) + uintptr(index)*elemSize)
	return (*leafPageElement)(ptr)
}

func (p *page) leafPageElements() []leafPageElement {
	if p.count == 0 {
		return nil
	}
	var res []leafPageElement
	for i := uint16(0); i < p.count; i++ {
		res = append(res, *p.leafPageElement(i))
	}
	return res
}

func (p *page) branchPageElement(index uint16) *branchPageElement {
	n := &branchPageElement{}
	elemSize := unsafe.Sizeof(*n)
	ptr := unsafe.Pointer(uintptr(unsafe.Pointer(&p.ptr)) + uintptr(index)*elemSize)
	return (*branchPageElement)(ptr)
}

func (p *page) branchPageElements() []branchPageElement {
	if p.count == 0 {
		return nil
	}
	var res []branchPageElement
	for i := uint16(0); i < p.count; i++ {
		res = append(res, *p.branchPageElement(i))
	}
	return res
}

type leafPageElement struct {
	flags uint32
	pos   uint32
	ksize uint32
	vsize uint32
}

func (n *leafPageElement) key() []byte {
	buf := (*[1 << 30]byte)(unsafe.Pointer(n))
	return (*[1 << 30]byte)(unsafe.Pointer(&buf[n.pos]))[:n.ksize:n.ksize]
}

func (n *leafPageElement) value() []byte {
	buf := (*[1 << 30]byte)(unsafe.Pointer(n))
	return (*[1 << 30]byte)(unsafe.Pointer(&buf[n.pos+n.ksize]))[:n.vsize:n.vsize]
}

type branchPageElement struct {
	pos   uint32
	ksize uint32
	pgid  pgid
}

func (n *branchPageElement) key() []byte {
	buf := (*[1 << 30]byte)(unsafe.Pointer(n))
	return (*[1 << 30]byte)(unsafe.Pointer(&buf[n.pos]))[:n.ksize:n.ksize]
}
