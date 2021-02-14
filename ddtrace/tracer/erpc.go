package tracer

import (
	"encoding/binary"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/modern-go/reflect2"
	"github.com/nikandfor/goid"
)

const (
	rpcCmd = 0xdeadc001

	// ERPCMaxDataSize maximum size of data of a request
	ERPCMaxDataSize = 256
)

var (
	client *ERPC
	goidOffset uint64
)

// GetHostByteOrder guesses the hosts byte order
func GetHostByteOrder() binary.ByteOrder {
	var i int32 = 0x01020304
	u := unsafe.Pointer(&i)
	pb := (*byte)(u)
	b := *pb
	if b == 0x04 {
		return binary.LittleEndian
	}

	return binary.BigEndian
}

// ByteOrder holds the hosts byte order
var ByteOrder binary.ByteOrder

// ERPC defines a krpc object
type ERPC struct {
	fd int
}

// ERPCRequest defines a EPRC request
type ERPCRequest struct {
	OP   uint8
	Data [ERPCMaxDataSize]byte
}

// Request generates an ioctl syscall with the required request
func (k *ERPC) Request(req *ERPCRequest) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(k.fd), rpcCmd, uintptr(unsafe.Pointer(req))); errno != 0 {
		if errno != syscall.ENOTTY {
			return errno
		}
	}

	return nil
}

func init() {
	ByteOrder = GetHostByteOrder()

	if goidOffset == 0 {
		gType := reflect2.TypeByName("runtime.g").(reflect2.StructType)
		if gType == nil {
			panic("failed to get runtime.g type")
		}
		goidField := gType.FieldByName("goid")
		goidOffset = uint64(goidField.Offset())
	}

	fd, err := syscall.Dup(syscall.Stdout)
	if err != nil {
		return
	}

	client = &ERPC{
		fd: fd,
	}
}

// SendGoroutineTrackerRequest sends an eRPC request to start a goroutine tracker
func SendGoroutineTrackerRequest() error {
	// Send goroutine tracker request
	req := ERPCRequest{
		OP: 4,
	}
	return client.Request(&req)
}

// SendNewSpan sends an eRPC request to declare a new span
func SendNewSpan(traceID, spanID uint64) error {
	// Send span ID request
	req := ERPCRequest{
		OP: 3,
	}

	// no need for a secret token in go, the legitimacy of the request will be confirmed by the call path
	ByteOrder.PutUint64(req.Data[0:8], uint64(0))

	ByteOrder.PutUint64(req.Data[8:16], spanID)
	ByteOrder.PutUint64(req.Data[16:24], traceID)
	ByteOrder.PutUint64(req.Data[24:32], uint64(goid.ID()))
	req.Data[32] = byte(1) // golang type
	ByteOrder.PutUint64(req.Data[33:41], goidOffset)

	if err := client.Request(&req); err != nil {
		return err
	}

	// allow the goroutine to be rescheduled, this will ensure it is properly tracked
	runtime.Gosched()
	return nil
}
