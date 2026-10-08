// 极简 msgpack 解码器（只读 datapack 模块所需类型，无编码）。
package dob

import (
	"encoding/binary"
	"fmt"
	"math"
)

// MsgpackDecode 解码一段 msgpack（map 键一律转 string，与 Python strict_map_key=False 对齐）。
func MsgpackDecode(data []byte) (any, error) {
	d := &msgpackDecoder{data: data}
	v, err := d.value()
	if err != nil {
		return nil, err
	}
	return v, nil
}

type msgpackDecoder struct {
	data []byte
	pos  int
}

func (d *msgpackDecoder) byte() (byte, error) {
	if d.pos >= len(d.data) {
		return 0, fmt.Errorf("msgpack: unexpected EOF")
	}
	b := d.data[d.pos]
	d.pos++
	return b, nil
}

func (d *msgpackDecoder) bytes(n int) ([]byte, error) {
	if d.pos+n > len(d.data) {
		return nil, fmt.Errorf("msgpack: unexpected EOF")
	}
	b := d.data[d.pos : d.pos+n]
	d.pos += n
	return b, nil
}

func (d *msgpackDecoder) uint(n int) (uint64, error) {
	b, err := d.bytes(n)
	if err != nil {
		return 0, err
	}
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	return v, nil
}

func (d *msgpackDecoder) value() (any, error) {
	b, err := d.byte()
	if err != nil {
		return nil, err
	}
	switch {
	case b <= 0x7f:
		return float64(b), nil
	case b >= 0xe0:
		return float64(int8(b)), nil
	case b >= 0xa0 && b <= 0xbf:
		return d.str(int(b & 0x1f))
	case b >= 0x90 && b <= 0x9f:
		return d.array(int(b & 0x0f))
	case b >= 0x80 && b <= 0x8f:
		return d.mapv(int(b & 0x0f))
	}
	switch b {
	case 0xc0:
		return nil, nil
	case 0xc2:
		return false, nil
	case 0xc3:
		return true, nil
	case 0xca:
		raw, err := d.bytes(4)
		if err != nil {
			return nil, err
		}
		return float64(math.Float32frombits(binary.BigEndian.Uint32(raw))), nil
	case 0xcb:
		raw, err := d.bytes(8)
		if err != nil {
			return nil, err
		}
		return math.Float64frombits(binary.BigEndian.Uint64(raw)), nil
	case 0xcc:
		v, err := d.uint(1)
		return float64(v), err
	case 0xcd:
		v, err := d.uint(2)
		return float64(v), err
	case 0xce:
		v, err := d.uint(4)
		return float64(v), err
	case 0xcf:
		v, err := d.uint(8)
		if err != nil {
			return nil, err
		}
		if v > 9007199254740991 {
			return float64(v), nil
		}
		return float64(v), nil
	case 0xd0:
		raw, err := d.bytes(1)
		return float64(int8(raw[0])), err
	case 0xd1:
		raw, err := d.bytes(2)
		return float64(int16(binary.BigEndian.Uint16(raw))), err
	case 0xd2:
		raw, err := d.bytes(4)
		return float64(int32(binary.BigEndian.Uint32(raw))), err
	case 0xd3:
		raw, err := d.bytes(8)
		return float64(int64(binary.BigEndian.Uint64(raw))), err
	case 0xd9:
		n, err := d.uint(1)
		if err != nil {
			return nil, err
		}
		return d.str(int(n))
	case 0xda:
		n, err := d.uint(2)
		if err != nil {
			return nil, err
		}
		return d.str(int(n))
	case 0xdb:
		n, err := d.uint(4)
		if err != nil {
			return nil, err
		}
		return d.str(int(n))
	case 0xc4:
		n, err := d.uint(1)
		if err != nil {
			return nil, err
		}
		return d.bytes(int(n))
	case 0xc5:
		n, err := d.uint(2)
		if err != nil {
			return nil, err
		}
		return d.bytes(int(n))
	case 0xc6:
		n, err := d.uint(4)
		if err != nil {
			return nil, err
		}
		return d.bytes(int(n))
	case 0xdc:
		n, err := d.uint(2)
		if err != nil {
			return nil, err
		}
		return d.array(int(n))
	case 0xdd:
		n, err := d.uint(4)
		if err != nil {
			return nil, err
		}
		return d.array(int(n))
	case 0xde:
		n, err := d.uint(2)
		if err != nil {
			return nil, err
		}
		return d.mapv(int(n))
	case 0xdf:
		n, err := d.uint(4)
		if err != nil {
			return nil, err
		}
		return d.mapv(int(n))
	default:
		return nil, fmt.Errorf("msgpack: unsupported type 0x%x", b)
	}
}

func (d *msgpackDecoder) str(n int) (any, error) {
	raw, err := d.bytes(n)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}

func (d *msgpackDecoder) array(n int) (any, error) {
	out := make([]any, 0, n)
	for i := 0; i < n; i++ {
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (d *msgpackDecoder) mapv(n int) (any, error) {
	out := make(map[string]any, n)
	for i := 0; i < n; i++ {
		k, err := d.value()
		if err != nil {
			return nil, err
		}
		v, err := d.value()
		if err != nil {
			return nil, err
		}
		var key string
		switch t := k.(type) {
		case string:
			key = t
		default:
			key = fmt.Sprint(t)
		}
		out[key] = v
	}
	return out, nil
}
