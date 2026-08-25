package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"unsafe"

	"go.etcd.io/bbolt"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "check":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: bbolt check <db-path>")
			os.Exit(1)
		}
		os.Exit(runCheck(os.Args[2]))
	case "info":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: bbolt info <db-path>")
			os.Exit(1)
		}
		os.Exit(runInfo(os.Args[2]))
	case "page":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: bbolt page <db-path> <pgid>")
			os.Exit(1)
		}
		pgid, err := strconv.ParseUint(os.Args[3], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid pgid: %v\n", err)
			os.Exit(1)
		}
		os.Exit(runPage(os.Args[2], pgid))
	case "dump":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "Usage: bbolt dump <db-path> <pgid>")
			os.Exit(1)
		}
		pgid, err := strconv.ParseUint(os.Args[3], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid pgid: %v\n", err)
			os.Exit(1)
		}
		os.Exit(runDump(os.Args[2], pgid))
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: bbolt <command> [arguments]")
	fmt.Println("Commands:")
	fmt.Println("  check <db-path>          Verify database integrity")
	fmt.Println("  info <db-path>           Print database summary information")
	fmt.Println("  page <db-path> <pgid>    Inspect a specific database page")
	fmt.Println("  dump <db-path> <pgid>    Hex dump a database page")
}

func runCheck(path string) int {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{ReadOnly: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		return 1
	}
	defer db.Close()

	activeMeta, tornIndex, warning, err := db.ValidateMeta()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		return 1
	}

	if tornIndex >= 0 {
		fmt.Fprintf(os.Stderr, "WARNING: %s\n", warning)
	}

	errs := db.Check()
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", e)
		}
		return 1
	}

	fmt.Printf("OK: Database is consistent at txid %d\n", activeMeta.Txid())
	return 0
}

func runInfo(path string) int {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{ReadOnly: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		return 1
	}
	defer db.Close()

	activeMeta, tornIndex, warning, err := db.ValidateMeta()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", err)
		return 1
	}

	if tornIndex >= 0 {
		fmt.Printf("Warning: %s\n", warning)
	}

	fmt.Printf("Page Size:       %d\n", activeMeta.PageSize())
	fmt.Printf("Total Pages:     %d\n", activeMeta.Pgid())
	fmt.Printf("TxID:            %d\n", activeMeta.Txid())
	fmt.Printf("Freelist Page:   %d\n", activeMeta.Freelist())
	return 0
}

func runPage(path string, id uint64) int {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		return 1
	}
	pageSize := bbolt.DefaultPageSize
	offset := int(id) * pageSize
	if offset >= len(data) {
		fmt.Fprintf(os.Stderr, "Page %d out of bounds (file size: %d)\n", id, len(data))
		return 1
	}

	if id == 0 || id == 1 {
		fmt.Printf("Page ID: %d (Meta Page)\n", id)
		return 0
	}

	p := (*pageHeader)(unsafe.Pointer(&data[offset]))
	fmt.Printf("Page ID: %d, Flags: 0x%04x, Count: %d, Overflow: %d\n", p.id, p.flags, p.count, p.overflow)
	return 0
}

type pageHeader struct {
	id       uint64
	flags    uint16
	count    uint16
	overflow uint32
}

func runDump(path string, id uint64) int {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		return 1
	}
	pageSize := bbolt.DefaultPageSize
	offset := int(id) * pageSize
	if offset >= len(data) {
		fmt.Fprintf(os.Stderr, "Page %d out of bounds\n", id)
		return 1
	}
	end := offset + pageSize
	if end > len(data) {
		end = len(data)
	}
	fmt.Print(hex.Dump(data[offset:end]))
	return 0
}
