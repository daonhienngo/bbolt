package bbolt

import (
	"bytes"
	"sort"
	"unsafe"
)

type Tx struct {
	db       *DB
	meta     *meta
	writable bool
	pages    map[pgid]*page
	dirty    map[pgid][]byte
	closed   bool
}

func (tx *Tx) DB() *DB {
	return tx.db
}

func (tx *Tx) Writable() bool {
	return tx.writable
}

func (tx *Tx) Bucket(name []byte) *Bucket {
	rootP := tx.page(tx.meta.root.root)
	if rootP == nil {
		return nil
	}
	return &Bucket{tx: tx, root: tx.meta.root.root, name: name}
}

func (tx *Tx) page(id pgid) *page {
	if p, ok := tx.pages[id]; ok {
		return p
	}
	if d, ok := tx.dirty[id]; ok {
		p := (*page)(unsafe.Pointer(&d[0]))
		tx.pages[id] = p
		return p
	}
	p := tx.db.page(id)
	if p != nil {
		tx.pages[id] = p
	}
	return p
}

func (tx *Tx) allocate(count int) (pgid, []byte) {
	id := tx.db.allocPgid
	tx.db.allocPgid += pgid(count)
	buf := make([]byte, count*tx.db.pageSize)
	p := (*page)(unsafe.Pointer(&buf[0]))
	p.id = id
	p.flags = leafPageFlag
	p.count = 0
	tx.dirty[id] = buf
	tx.pages[id] = p
	return id, buf
}

func (tx *Tx) Commit() error {
	if tx.closed {
		return ErrTxClosed
	}
	if !tx.writable {
		return ErrTxNotWritable
	}
	tx.closed = true

	// Write dirty data pages first
	var pgids []pgid
	for id := range tx.dirty {
		pgids = append(pgids, id)
	}
	sort.Slice(pgids, func(i, j int) bool { return pgids[i] < pgids[j] })

	for _, id := range pgids {
		buf := tx.dirty[id]
		offset := int64(id) * int64(tx.db.pageSize)
		if _, err := tx.db.file.WriteAt(buf, offset); err != nil {
			return err
		}
	}

	// Sync dirty data pages to disk before meta page update
	if err := tx.db.file.Sync(); err != nil {
		return err
	}

	// Alternate meta page write
	nextTxid := tx.meta.txid + 1
	metaIdx := int(nextTxid % 2)
	metaBuf := make([]byte, tx.db.pageSize)
	metaPage := (*page)(unsafe.Pointer(&metaBuf[0]))
	metaPage.id = pgid(metaIdx)
	metaPage.flags = metaPageFlag

	m := metaPage.meta()
	m.magic = magic
	m.version = Version
	m.pageSize = uint32(tx.db.pageSize)
	m.flags = 0
	m.root = tx.meta.root
	m.freelist = tx.meta.freelist
	m.pgid = tx.db.allocPgid
	m.txid = nextTxid
	m.checksum = m.sum64()

	// Write meta page
	offset := int64(metaIdx) * int64(tx.db.pageSize)
	if _, err := tx.db.file.WriteAt(metaBuf, offset); err != nil {
		return err
	}
	if err := tx.db.file.Sync(); err != nil {
		return err
	}

	return tx.db.readAll()
}

func (tx *Tx) Rollback() error {
	tx.closed = true
	tx.dirty = nil
	tx.pages = nil
	return nil
}

type Bucket struct {
	tx   *Tx
	root pgid
	name []byte
}

func (b *Bucket) Get(key []byte) []byte {
	p := b.tx.page(b.root)
	if p == nil {
		return nil
	}
	return b.search(p, key)
}

func (b *Bucket) search(p *page, key []byte) []byte {
	if (p.flags & leafPageFlag) != 0 {
		elems := p.leafPageElements()
		for i := range elems {
			elem := &elems[i]
			if bytes.Equal(elem.key(), key) {
				return elem.value()
			}
		}
		return nil
	}
	if (p.flags & branchPageFlag) != 0 {
		elems := p.branchPageElements()
		for i := len(elems) - 1; i >= 0; i-- {
			elem := &elems[i]
			if bytes.Compare(key, elem.key()) >= 0 {
				child := b.tx.page(elem.pgid)
				if child != nil {
					return b.search(child, key)
				}
			}
		}
	}
	return nil
}

func (b *Bucket) Put(key, val []byte) error {
	if len(key) == 0 {
		return ErrKeyRequired
	}
	if !b.tx.writable {
		return ErrTxNotWritable
	}

	p := b.tx.page(b.root)
	buf := make([]byte, b.tx.db.pageSize)
	copy(buf, (*[1 << 30]byte)(unsafe.Pointer(p))[:b.tx.db.pageSize:b.tx.db.pageSize])
	newPage := (*page)(unsafe.Pointer(&buf[0]))

	// Extract existing elements
	type item struct {
		k []byte
		v []byte
	}
	var items []item
	found := false
	for _, elem := range p.leafPageElements() {
		if bytes.Equal(elem.key(), key) {
			items = append(items, item{k: key, v: val})
			found = true
		} else {
			items = append(items, item{k: elem.key(), v: elem.value()})
		}
	}
	if !found {
		items = append(items, item{k: key, v: val})
	}
	sort.Slice(items, func(i, j int) bool { return bytes.Compare(items[i].k, items[j].k) < 0 })

	// Write items to new page
	newPage.id = b.root
	newPage.flags = leafPageFlag
	newPage.count = uint16(len(items))

	elemHeaderSize := int(unsafe.Sizeof(leafPageElement{}))
	pos := len(buf)
	for i, it := range items {
		pos -= (len(it.k) + len(it.v))
		copy(buf[pos:], it.k)
		copy(buf[pos+len(it.k):], it.v)
		elem := newPage.leafPageElement(uint16(i))
		elem.flags = 0
		elem.ksize = uint32(len(it.k))
		elem.vsize = uint32(len(it.v))
		elem.pos = uint32(pos - (pageHeaderSize + i*elemHeaderSize))
	}

	b.tx.dirty[b.root] = buf
	b.tx.pages[b.root] = newPage
	return nil
}

func (b *Bucket) ForEach(fn func(k, v []byte) error) error {
	p := b.tx.page(b.root)
	if p == nil {
		return nil
	}
	for _, elem := range p.leafPageElements() {
		if err := fn(elem.key(), elem.value()); err != nil {
			return err
		}
	}
	return nil
}
