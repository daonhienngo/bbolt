package bbolt

import (
	"fmt"
)

func (db *DB) Check() []error {
	var errs []error

	activeMeta, tornIndex, _, metaErr := db.ValidateMeta()
	if metaErr != nil {
		return []error{metaErr}
	}

	if tornIndex >= 0 {
		// Note: a torn inactive meta page is a recoverable diagnostic warning, not an integrity error.
	}

	reachable := make(map[pgid]bool)
	// Traverse reachable pages from active root bucket
	if activeMeta.root.root != 0 {
		db.checkPage(activeMeta.root.root, activeMeta, reachable, &errs)
	}

	// Traverse freelist pages
	if activeMeta.freelist != 0 && activeMeta.freelist < activeMeta.pgid {
		reachable[activeMeta.freelist] = true
		fp := db.page(activeMeta.freelist)
		if fp == nil {
			errs = append(errs, fmt.Errorf("freelist page %d not found", activeMeta.freelist))
		} else if (fp.flags & freelistPageFlag) == 0 {
			errs = append(errs, fmt.Errorf("page %d is not a freelist page (flags: %02x)", activeMeta.freelist, fp.flags))
		}
	}

	// Verify all free pages are strictly within active high-water mark
	for _, freePgid := range db.freelist.ids {
		if freePgid >= activeMeta.pgid {
			errs = append(errs, fmt.Errorf("freelist pgid %d exceeds active high-water mark %d", freePgid, activeMeta.pgid))
		}
		if reachable[freePgid] && freePgid != activeMeta.freelist {
			errs = append(errs, fmt.Errorf("page %d is both reachable and marked free", freePgid))
		}
	}

	return errs
}

func (db *DB) checkPage(id pgid, activeMeta *meta, reachable map[pgid]bool, errs *[]error) {
	if id >= activeMeta.pgid {
		*errs = append(*errs, fmt.Errorf("page %d exceeds active meta high-water mark %d", id, activeMeta.pgid))
		return
	}
	if reachable[id] {
		*errs = append(*errs, fmt.Errorf("cycle detected at page %d", id))
		return
	}
	reachable[id] = true

	p := db.page(id)
	if p == nil {
		*errs = append(*errs, fmt.Errorf("page %d is unreachable / out of bounds", id))
		return
	}

	if (p.flags & branchPageFlag) != 0 {
		for _, elem := range p.branchPageElements() {
			db.checkPage(elem.pgid, activeMeta, reachable, errs)
		}
	} else if (p.flags & leafPageFlag) != 0 {
		// Leaf page elements are valid
	} else if (p.flags & metaPageFlag) != 0 {
		*errs = append(*errs, fmt.Errorf("unexpected meta page %d in B-tree", id))
	}
}
