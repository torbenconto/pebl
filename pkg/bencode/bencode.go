package bencode

import (
	"crypto/sha1"
	"errors"
	"strconv"
	"unicode"
)

func DecodeWithInfoHash(data []byte) (interface{}, []byte, [20]byte, error) {
	pos := 0
	val, infoRaw, err := decodeTopLevelWithInfo(data, &pos)
	var infoHash [20]byte
	if err == nil && infoRaw != nil {
		infoHash = sha1.Sum(infoRaw)
	}
	return val, infoRaw, infoHash, err
}

func decodeTopLevelWithInfo(data []byte, pos *int) (interface{}, []byte, error) {
	if *pos >= len(data) || data[*pos] != 'd' {
		return nil, nil, errors.New("top-level must be a dictionary")
	}
	*pos++
	dict := make(map[string]interface{})
	var infoRaw []byte

	for *pos < len(data) && data[*pos] != 'e' {
		keyRaw, err := decodeAt(data, pos)
		if err != nil {
			return nil, nil, err
		}
		keyStr, ok := keyRaw.(string)
		if !ok {
			return nil, nil, errors.New("dictionary key is not a string")
		}

		if keyStr == "info" {
			infoStart := *pos
			_, err := decodeAt(data, pos)
			if err != nil {
				return nil, nil, err
			}
			infoRaw = data[infoStart:*pos]

			tmpPos := infoStart
			infoVal, err := decodeAt(data, &tmpPos)
			if err != nil {
				return nil, nil, err
			}
			dict[keyStr] = infoVal
		} else {
			val, err := decodeAt(data, pos)
			if err != nil {
				return nil, nil, err
			}
			dict[keyStr] = val
		}
	}

	if *pos >= len(data) || data[*pos] != 'e' {
		return nil, nil, errors.New("unterminated dictionary")
	}
	*pos++
	return dict, infoRaw, nil
}

func Decode(data []byte) (interface{}, error) {
	pos := 0
	return decodeAt(data, &pos)
}

func decodeAt(data []byte, pos *int) (interface{}, error) {
	if *pos >= len(data) {
		return nil, errors.New("unexpected end of data")
	}

	switch data[*pos] {
	case 'i':
		*pos++
		start := *pos
		for *pos < len(data) && data[*pos] != 'e' {
			*pos++
		}
		if *pos >= len(data) {
			return nil, errors.New("unterminated integer")
		}
		numStr := string(data[start:*pos])
		*pos++
		return strconv.Atoi(numStr)

	case 'l':
		*pos++
		var list []interface{}
		for *pos < len(data) && data[*pos] != 'e' {
			item, err := decodeAt(data, pos)
			if err != nil {
				return nil, err
			}
			list = append(list, item)
		}
		if *pos >= len(data) || data[*pos] != 'e' {
			return nil, errors.New("unterminated list")
		}
		*pos++
		return list, nil

	case 'd':
		*pos++
		dict := make(map[string]interface{})
		for *pos < len(data) && data[*pos] != 'e' {
			keyRaw, err := decodeAt(data, pos)
			if err != nil {
				return nil, err
			}
			keyStr, ok := keyRaw.(string)
			if !ok {
				return nil, errors.New("dictionary key is not a string")
			}
			val, err := decodeAt(data, pos)
			if err != nil {
				return nil, err
			}
			dict[keyStr] = val
		}
		if *pos >= len(data) || data[*pos] != 'e' {
			return nil, errors.New("unterminated dictionary")
		}
		*pos++
		return dict, nil

	default:
		if !unicode.IsDigit(rune(data[*pos])) {
			return nil, errors.New("unexpected character: expected digit")
		}
		start := *pos
		for *pos < len(data) && data[*pos] != ':' {
			*pos++
		}
		if *pos >= len(data) {
			return nil, errors.New("missing ':' in string")
		}
		length, err := strconv.Atoi(string(data[start:*pos]))
		if err != nil {
			return nil, err
		}
		*pos++
		if *pos+length > len(data) {
			return nil, errors.New("string out of bounds")
		}
		str := string(data[*pos : *pos+length])
		*pos += length
		return str, nil
	}
}
