package micro

import (
	"encoding/binary"
)

/*
fixed header:
00 length
04 Flag
08 Id
10 extlen
14 bodylen

header ext:
18 extlen bytes
X0 bytes of bodylen

request:
Y0 namelen
Y4 bytes of namelen

response:
Y0 code
*/

const headerSize = 4 + 8 + 4 + 4

const (
	FlagReply = 1
)

type Packet struct {
	done int32
	Flag uint32
	Code int32
	Id   uint64
	Name string
	Body []byte
	Ext  []byte
}

func EncodePacket(dst []byte, src *Packet) []byte {
	n := 4 + headerSize + len(src.Ext) + len(src.Body) + 4
	if src.Flag&FlagReply == 0 {
		n += len(src.Name)
	}
	if cap(dst)-len(dst) < n {
		data := make([]byte, len(dst), len(dst)+n)
		copy(data, dst)
		dst = data
	}
	data := dst[len(dst) : len(dst)+n]
	_ = data[23]
	binary.LittleEndian.PutUint32(data[0:4], uint32(n-4))
	binary.LittleEndian.PutUint32(data[4:8], src.Flag)
	binary.LittleEndian.PutUint64(data[8:16], src.Id)
	binary.LittleEndian.PutUint32(data[16:20], uint32(len(src.Ext)))
	binary.LittleEndian.PutUint32(data[20:24], uint32(len(src.Body)))
	p := 24 + copy(data[24:], src.Ext)
	p += copy(data[p:], src.Body)
	if src.Flag&FlagReply == 0 {
		binary.LittleEndian.PutUint32(data[p:], uint32(len(src.Name)))
		copy(data[p+4:], src.Name)
	} else {
		binary.LittleEndian.PutUint32(data[p:], uint32(src.Code))
	}
	return dst[:len(dst)+n]
}

func DecodePacket(dst *Packet, src []byte) int {
	if len(src) < headerSize {
		return 0
	}
	_ = src[19]
	dst.Flag = binary.LittleEndian.Uint32(src[0:4])
	dst.Id = binary.LittleEndian.Uint64(src[4:12])
	extlen := binary.LittleEndian.Uint32(src[12:16])
	bodylen := binary.LittleEndian.Uint32(src[16:20])
	o := 20 + extlen
	n := int(o + bodylen + 4)
	if len(src) < n {
		return 0
	}
	if dst.Flag&FlagReply == 0 {
		namelen := int(binary.LittleEndian.Uint32(src[n-4 : n]))
		if len(src) < n+namelen {
			return 0
		}
		dst.Name = string(src[n : n+namelen])
		n += namelen
	} else {
		dst.Code = int32(binary.LittleEndian.Uint32(src[n-4 : n]))
	}
	dst.Ext = src[20:o]
	dst.Body = src[o : o+bodylen]
	return n
}
