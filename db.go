package bbolt

import (
	"fmt"
	"os"
	"sync"
	"unsafe"
)

const DefaultPageSize = 4096

type Options struct {
	PageSize     int
	FreelistType FreelistType
	ReadOnly     bool
}

type DB struct {
	path     string
	file     *os.File
	pageSize int
	data     []byte
	meta0    *meta
	meta1    *meta
	freelist *freelist
	roLock   sync.RWMutex
	rwLock   sync.Mutex
	opened   bool
	readOnly bool
	allocPgid pgid
}

func Open(path string, mode os.FileMode, options *Options) (*DB, error) {
	if options == nil {
		options = &Options{PageSize: DefaultPageSize}
	}
	pageSize := options.PageSize
	if pageSize == 0 {
		pageSize = DefaultPageSize
	}

	flag := os.O_RDWR | os.O_CREATE
	if options.ReadOnly {
		flag = os.O_RDONLY
	}

	f, err := os.OpenFile(path, flag, mode)
	if err != nil {
		return nil, err
	}

	db := &DB{
		path:     path,
		file:     f,
		pageSize: pageSize,
		freelist: newFreelist(options.FreelistType),
		opened:   true,
		readOnly: options.ReadOnly,
	}

	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	if fi.Size() == 0 {
		if err := db.init(pageSize); err != nil {
			_ = f.Close()
			return nil, err
		}
	} else {
		if err := db.readAll(); err != nil {
			_ = f.Close()
			return nil, err
		}
	}

	activeMeta, _, _, err := db.ValidateMeta()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	db.pageSize = int(activeMeta.pageSize)
	db.allocPgid = activeMeta.pgid
	if activeMeta.freelist != 0 && activeMeta.freelist < activeMeta.pgid {
		db.freelist.read(db.page(activeMeta.freelist))
	}

	return db, nil
}

func (db *DB) init(pageSize int) error {
	db.pageSize = pageSize
	buf := make([]byte, pageSize*4)

	for i := 0; i < 2; i++ {
		p := (*page)(unsafe.Pointer(&buf[i*pageSize]))
		p.id = pgid(i)
		p.flags = metaPageFlag
		m := p.meta()
		m.magic = magic
		m.version = Version
		m.pageSize = uint32(pageSize)
		m.freelist = 2
		m.pgid = 4
		m.txid = txid(i)
		m.checksum = m.sum64()
	}

	freep := (*page)(unsafe.Pointer(&buf[2*pageSize]))
	freep.id = 2
	freep.flags = freelistPageFlag
	freep.count = 0

	rootp := (*page)(unsafe.Pointer(&buf[3*pageSize]))
	rootp.id = 3
	rootp.flags = leafPageFlag
	rootp.count = 0

	for i := 0; i < 2; i++ {
		p := (*page)(unsafe.Pointer(&buf[i*pageSize]))
		m := p.meta()
		m.root = bucketHeader{root: 3, sequence: 0}
		m.checksum = m.sum64()
	}

	if _, err := db.file.WriteAt(buf, 0); err != nil {
		return err
	}
	if err := db.file.Sync(); err != nil {
		return err
	}
	db.data = buf
	db.allocPgid = 4
	return nil
}

func (db *DB) readAll() error {
	fi, err := db.file.Stat()
	if err != nil {
		return err
	}
	db.data = make([]byte, fi.Size())
	if _, err := db.file.ReadAt(db.data, 0); err != nil && fi.Size() > 0 {
		return err
	}
	return nil
}

func (db *DB) Close() error {
	db.rwLock.Lock()
	defer db.rwLock.Unlock()
	if !db.opened {
		return nil
	}
	db.opened = false
	return db.file.Close()
}

func (db *DB) page(id pgid) *page {
	offset := int(id) * db.pageSize
	if offset+pageHeaderSize > len(db.data) {
		return nil
	}
	return (*page)(unsafe.Pointer(&db.data[offset]))
}

func (db *DB) ValidateMeta() (active *meta, tornIndex int, warning string, err error) {
	if len(db.data) < db.pageSize*2 {
		return nil, -1, "", ErrInvalid
	}

	meta0 := (*page)(unsafe.Pointer(&db.data[0])).meta()
	meta1 := (*page)(unsafe.Pointer(&db.data[db.pageSize])).meta()

	err0 := meta0.validate()
	err1 := meta1.validate()

	if err0 != nil && err1 != nil {
		return nil, -1, "", fmt.Errorf("unrecoverable corruption: both meta pages invalid (meta0: %v, meta1: %v)", err0, err1)
	}

	if err0 == nil && err1 != nil {
		return meta0, 1, fmt.Sprintf("meta1 is torn or invalid (%v), bypassed in favor of valid meta0 (txid %d)", err1, meta0.txid), nil
	}

	if err0 != nil && err1 == nil {
		return meta1, 0, fmt.Sprintf("meta0 is torn or invalid (%v), bypassed in favor of valid meta1 (txid %d)", err0, meta1.txid), nil
	}

	// Both valid: select higher txid
	if meta1.txid > meta0.txid {
		return meta1, -1, "", nil
	}
	return meta0, -1, "", nil
}

func (db *DB) meta() *meta {
	m, _, _, _ := db.ValidateMeta()
	return m
}

func (db *DB) View(fn func(tx *Tx) error) error {
	db.roLock.RLock()
	defer db.roLock.RUnlock()
	if !db.opened {
		return ErrDatabaseNotOpen
	}
	tx := &Tx{db: db, meta: db.meta(), writable: false, pages: make(map[pgid]*page)}
	return fn(tx)
}

func (db *DB) Update(fn func(tx *Tx) error) error {
	db.rwLock.Lock()
	defer db.rwLock.Unlock()
	if !db.opened {
		return ErrDatabaseNotOpen
	}
	if db.readOnly {
		return ErrTxNotWritable
	}
	if err := db.readAll(); err != nil {
		return err
	}
	activeMeta := db.meta()
	tx := &Tx{
		db:       db,
		meta:     activeMeta,
		writable: true,
		pages:    make(map[pgid]*page),
		dirty:    make(map[pgid][]byte),
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (db *DB) Path() string {
	return db.path
}
