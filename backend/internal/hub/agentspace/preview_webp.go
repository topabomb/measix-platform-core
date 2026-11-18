package agentspace

import "encoding/binary"

// validPreviewWebP checks the bounded container and one still-image header. It
// never allocates decoded pixels or claims to validate the compressed pixels.
// Format: https://developers.google.com/speed/webp/docs/riff_container and
// https://developers.google.com/speed/webp/docs/webp_lossless_bitstream_specification
func validPreviewWebP(data []byte, size int) bool {
	if len(data) < 20 || len(data) > maxPreviewBytes || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" || uint64(binary.LittleEndian.Uint32(data[4:8])) != uint64(len(data)-8) {
		return false
	}
	canvasWidth, canvasHeight, width, height := 0, 0, 0, 0
	extended, alpha, image := false, false, false
	var features byte
	for offset := 12; offset < len(data); {
		if len(data)-offset < 8 {
			return false
		}
		kind := string(data[offset : offset+4])
		n := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		if n+n%2 > uint64(len(data)-offset-8) {
			return false
		}
		payload := data[offset+8 : offset+8+int(n)]
		if n%2 != 0 && data[offset+8+int(n)] != 0 {
			return false
		}
		switch kind {
		case "VP8X":
			if offset != 12 || extended || len(payload) != 10 || payload[0]&0x02 != 0 {
				return false
			}
			extended = true
			features = payload[0]
			canvasWidth = 1 + previewUint24(payload[4:7])
			canvasHeight = 1 + previewUint24(payload[7:10])
			if canvasWidth > size || canvasHeight > size {
				return false
			}
		case "ANIM", "ANMF":
			return false
		case "ALPH":
			if !extended || image || alpha || len(payload) < 2 || features&0x10 == 0 {
				return false
			}
			compression := payload[0] & 3
			if compression > 1 || (payload[0]>>4)&3 > 1 || compression == 0 && len(payload)-1 != canvasWidth*canvasHeight {
				return false
			}
			alpha = true
		case "VP8 ":
			if image || len(payload) < 11 || payload[0]&1 != 0 || payload[0]&0x10 == 0 || (payload[0]>>1)&7 > 3 || string(payload[3:6]) != "\x9d\x01\x2a" {
				return false
			}
			partition := previewUint24(payload[:3]) >> 5
			if partition < 1 || partition > len(payload)-10 || extended && features&0x10 != 0 && !alpha {
				return false
			}
			width = int(binary.LittleEndian.Uint16(payload[6:8]) & 0x3fff)
			height = int(binary.LittleEndian.Uint16(payload[8:10]) & 0x3fff)
			image = true
		case "VP8L":
			if image || alpha || len(payload) < 6 || payload[0] != 0x2f {
				return false
			}
			bits := binary.LittleEndian.Uint32(payload[1:5])
			if bits>>29 != 0 {
				return false
			}
			width = 1 + int(bits&0x3fff)
			height = 1 + int((bits>>14)&0x3fff)
			image = true
		case "ICCP":
			if !extended || image || alpha {
				return false
			}
		}
		offset += 8 + int(n+n%2)
	}
	return image && width > 0 && height > 0 && width <= size && height <= size && (!extended || width == canvasWidth && height == canvasHeight)
}

func previewUint24(value []byte) int { return int(value[0]) | int(value[1])<<8 | int(value[2])<<16 }
