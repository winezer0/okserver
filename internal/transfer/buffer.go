package transfer

import "sync"

const BufferSize = 32 << 10

var byteBuffers = sync.Pool{New: func() any {
	return make([]byte, BufferSize)
}}

func GetBuffer() []byte {
	return byteBuffers.Get().([]byte)
}

func PutBuffer(buf []byte) {
	if cap(buf) < BufferSize {
		return
	}
	byteBuffers.Put(buf[:BufferSize])
}
