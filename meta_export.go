package bbolt

// Public accessors on meta for inspection tools
func (m *meta) PageSize() int {
	return int(m.pageSize)
}

func (m *meta) Pgid() uint64 {
	return uint64(m.pgid)
}

func (m *meta) Txid() uint64 {
	return uint64(m.txid)
}

func (m *meta) Freelist() uint64 {
	return uint64(m.freelist)
}
