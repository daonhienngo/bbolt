package bbolt

import "errors"

var (
	// ErrDatabaseNotOpen is returned when an operation is performed on a closed database.
	ErrDatabaseNotOpen = errors.New("database not open")

	// ErrInvalid is returned when a database file is invalid or not a bbolt db.
	ErrInvalid = errors.New("invalid database")

	// ErrVersionMismatch is returned when the database version does not match.
	ErrVersionMismatch = errors.New("version mismatch")

	// ErrChecksum is returned when meta checksum verification fails.
	ErrChecksum = errors.New("checksum error")

	// ErrTxClosed is returned when an operation is performed on a closed transaction.
	ErrTxClosed = errors.New("tx closed")

	// ErrTxNotWritable is returned when an update operation is attempted on a read-only transaction.
	ErrTxNotWritable = errors.New("tx not writable")

	// ErrBucketNotFound is returned when a bucket does not exist.
	ErrBucketNotFound = errors.New("bucket not found")

	// ErrBucketExists is returned when creating a bucket that already exists.
	ErrBucketExists = errors.New("bucket already exists")

	// ErrKeyRequired is returned when an empty key is provided.
	ErrKeyRequired = errors.New("key required")
)
