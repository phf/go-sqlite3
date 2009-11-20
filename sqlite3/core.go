/*
	THIS IS NOT DONE AT ALL! USE AT YOUR OWN RISK!

	- it would be nice if cgo could grok several .go files,
	so far it can't; so all the C interface stuff has to be
	in one file; bummer that
*/

package sqlite3

/*
#include <stdlib.h>
#include "wrapper.h"
*/
import "C"
import "unsafe"

import "fmt"
import "os"
import "strconv"


/*
	These constants can be or'd together and passed as the
	"sqlite3.flags" argument to Open(). Some of them only
	apply if "sqlite3.vfs" is also passed. See the SQLite
	documentation for details.
*/
const (
	OpenReadOnly = 0x00000001;
	OpenReadWrite = 0x00000002;
	OpenCreate = 0x00000004;
	OpenDeleteOnClose = 0x00000008;  /* VFS only */
	OpenExclusive = 0x00000010;  /* VFS only */
	OpenMainDb = 0x00000100;  /* VFS only */
	OpenTempDb = 0x00000200;  /* VFS only */
	OpenTransientDb = 0x00000400;  /* VFS only */
	OpenMainJournal = 0x00000800;  /* VFS only */
	OpenTempJournal = 0x00001000;  /* VFS only */
	OpenSubJournal = 0x00002000;  /* VFS only */
	OpenMasterJournal = 0x00004000;  /* VFS only */
	OpenNoMutex = 0x00008000;
	OpenFullMutex = 0x00010000;
	OpenSharedCache = 0x00020000;
	OpenPrivateCache = 0x00040000;
)

/* after we run into a lock, we'll retry for this long */
const defaultTimeoutMilliseconds = 16*1000;

/* SQLite connections */
type Connection struct {
	/* pointer to struct sqlite3 */
	handle C.wsq_db;
}

/* SQLite cursors, will be renamed/refactored soon */
type Cursor struct {
	/* pointer to struct sqlite3_stmt */
	handle C.wsq_st;
	/* connection we were created on */
	connection *Connection;
	/* the last query yielded results */
	result bool;
}

/*
	The SQLite database interface returns keys "version",
	"sqlite3.sourceid", and "sqlite3.versionnumber"; the
	latter are specific to SQLite.
*/
func Version() (data map[string]string, error os.Error)
{
	data = make(map[string]string);

	cp := C.wsq_libversion();
	if (cp == nil) {
		error = &InterfaceError{"Version: couldn't get library version!"};
		return;
	}
	data["version"] = C.GoString(cp);
	// TODO: fake client and server keys?

	cp = C.wsq_sourceid();
	if (cp != nil) {
		data["sqlite3.sourceid"] = C.GoString(cp);
	}

	i := C.wsq_libversion_number();
	data["sqlite3.versionnumber"] = strconv.Itob(int(i), 10);

	return;
}

type Any interface{};
type ConnectionInfo map[string] Any;

func parseConnInfo(info ConnectionInfo) (name string, flags int, vfs *string, error os.Error)
{
	ok := false;
	any := Any(nil);

	any, ok = info["name"];
	if !ok {
		error = &InterfaceError{"Open: No \"name\" in arguments map."};
		return;
	}
	name, ok = any.(string);
	if !ok {
		error = &InterfaceError{"Open: \"name\" argument not a string."};
		return;
	}

	any, ok = info["sqlite.flags"];
	if ok {
		flags = any.(int);
	}

	any, ok = info["sqlite.vfs"];
	if ok {
		vfs = new(string);
		*vfs = any.(string);
	}

	return;
}

/* TODO: use URIs instead? http://golang.org/pkg/http/#URL */
func Open(info ConnectionInfo) (conn *Connection, error os.Error)
{
	name, flags, vfs, error := parseConnInfo(info);
	if error != nil {
		return;
	}

	conn = new(Connection);

	rc := StatusOk;
	p := C.CString(name);

	if vfs != nil {
		q := C.CString(*vfs);
		rc = int(C.wsq_open(p, &conn.handle, C.int(flags), q));
		C.free(unsafe.Pointer(q));
	}
	else {
		rc = int(C.wsq_open(p, &conn.handle, C.int(flags), nil));
	}

	C.free(unsafe.Pointer(p));
	if rc != StatusOk {
		error = conn.error();
	}
	else {
		rc := C.wsq_busy_timeout(conn.handle, defaultTimeoutMilliseconds);
		if rc != StatusOk {
			error = conn.error();
		}
	}

	return;
}

func (self *Connection) error() (error os.Error) {
	e := new(DatabaseError);
	e.basic = int(C.wsq_errcode(self.handle));
	e.extended = int(C.wsq_extended_errcode(self.handle));
	e.message = C.GoString(C.wsq_errmsg(self.handle));
	return e;
}

func (self *Connection) Cursor() (cursor *Cursor, error os.Error) {
	cursor = new(Cursor);
	cursor.connection = self;
	return;
}

func (self *Connection) Close() (error os.Error) {
	rc := C.wsq_close(self.handle);
	if rc != StatusOk {
		error = self.error();
	}
	return;
}

func (self *Cursor) Execute(query string, parameters ...) (error os.Error) {
	query = fmt.Sprintf(query, parameters);

	q := C.CString(query);

	rc := C.wsq_prepare(self.connection.handle, q, -1, &self.handle, nil);
	if rc != StatusOk {
		error = self.connection.error();
		if self.handle != nil {
			// TODO: finalize
		}
		return;
	}

	rc = C.wsq_step(self.handle);
	switch rc {
		case StatusDone:
			self.result = false;
			// TODO: finalize
		case StatusRow:
			self.result = true;
			// TODO: obtain results somehow? or later call?
		default:
			error = self.connection.error();
			// TODO: finalize
			return;
	}

	C.free(unsafe.Pointer(q));
	return;
}

func (self *Cursor) FetchOne() (data []interface{}, error os.Error) {
	if !self.result {
		error = &InterfaceError{"FetchOne: No results to fetch!"};
		return;
	}

	nColumns := int(C.wsq_column_count(self.handle));
	if nColumns <= 0 {
		error = &InterfaceError{"FetchOne: No columns in result!"};
		return;
	}

	data = make([]interface{}, nColumns);
	for i := 0; i < nColumns; i++ {
		text := C.wsq_column_text(self.handle, C.int(i));
		data[i] = C.GoString(text);
	}

	rc := C.wsq_step(self.handle);
	switch rc {
		case StatusDone:
			self.result = false;
			// TODO: finalize
		case StatusRow:
			self.result = true;
		default:
			error = self.connection.error();
			// TODO: finalize
			return;
	}

	return;
}
func (self *Cursor) FetchRow() (data map[string]interface{}, error os.Error) {
	if !self.result {
		error = &InterfaceError{"FetchRow: No results to fetch!"};
		return;
	}

	nColumns := int(C.wsq_column_count(self.handle));
	if nColumns <= 0 {
		error = &InterfaceError{"FetchRow: No columns in result!"};
		return;
	}

	data = make(map[string]interface{}, nColumns);
	for i := 0; i < nColumns; i++ {
		text := C.wsq_column_text(self.handle, C.int(i));
		name := C.wsq_column_name(self.handle, C.int(i));
		data[C.GoString(name)] = C.GoString(text);
	}

	rc := C.wsq_step(self.handle);
	switch rc {
		case StatusDone:
			self.result = false;
			// TODO: finalize
		case StatusRow:
			self.result = true;
		default:
			error = self.connection.error();
			// TODO: finalize
			return;
	}

	return;
}

func (self *Cursor) Close() (error os.Error) {
	if self.handle != nil {
		rc := C.wsq_finalize(self.handle);
		if rc != StatusOk {
			error = self.connection.error();
		}
	}
	return;
}