package schema

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ParseType parses a Glue type string into a DataType.
func ParseType(s string) (DataType, error) {
	p := &typeParser{input: s}
	dt, err := p.parse()
	if err != nil {
		return nil, err
	}
	p.skipWhitespace()
	if p.pos < len(p.input) {
		return nil, fmt.Errorf("unexpected characters at end of input: %s", p.input[p.pos:])
	}
	return dt, nil
}

type typeParser struct {
	input string
	pos   int
}

func (p *typeParser) parse() (DataType, error) {
	p.skipWhitespace()
	ident := p.readIdentifier()

	switch ident {
	case "string", "boolean", "tinyint", "smallint", "int", "bigint", "float", "double", "date", "timestamp", "binary":
		return PrimitiveType{Kind: PrimitiveKind(ident)}, nil
	case "decimal":
		return p.parseDecimal()
	case "array":
		return p.parseArray()
	case "map":
		return p.parseMap()
	case "struct":
		return p.parseStruct()
	case "":
		return nil, fmt.Errorf("expected type identifier at position %d", p.pos)
	default:
		return nil, fmt.Errorf("unknown type identifier: %s", ident)
	}
}

func (p *typeParser) parseDecimal() (DataType, error) {
	if err := p.expect("("); err != nil {
		return nil, err
	}
	precisionStr := p.readDigits()
	precision, _ := strconv.Atoi(precisionStr)
	if err := p.expect(","); err != nil {
		return nil, err
	}
	scaleStr := p.readDigits()
	scale, _ := strconv.Atoi(scaleStr)
	if err := p.expect(")"); err != nil {
		return nil, err
	}
	return DecimalType{Precision: precision, Scale: scale}, nil
}

func (p *typeParser) parseArray() (DataType, error) {
	if err := p.expect("<"); err != nil {
		return nil, err
	}
	elemType, err := p.parse()
	if err != nil {
		return nil, err
	}
	if err := p.expect(">"); err != nil {
		return nil, err
	}
	return ArrayType{ElementType: elemType}, nil
}

func (p *typeParser) parseMap() (DataType, error) {
	if err := p.expect("<"); err != nil {
		return nil, err
	}
	keyType, err := p.parse()
	if err != nil {
		return nil, err
	}
	if err := p.expect(","); err != nil {
		return nil, err
	}
	valueType, err := p.parse()
	if err != nil {
		return nil, err
	}
	if err := p.expect(">"); err != nil {
		return nil, err
	}
	return MapType{KeyType: keyType, ValueType: valueType}, nil
}

func (p *typeParser) parseStruct() (DataType, error) {
	if err := p.expect("<"); err != nil {
		return nil, err
	}
	var fields []StructField
	for {
		p.skipWhitespace()
		fieldName := p.readFieldName()
		if fieldName == "" {
			return nil, fmt.Errorf("expected field name in struct at position %d", p.pos)
		}
		if err := p.expect(":"); err != nil {
			return nil, err
		}
		
		typeStart := p.pos
		fieldType, err := p.parse()
		if err != nil {
			return nil, err
		}
		typeEnd := p.pos
		
		fields = append(fields, StructField{
			Name:       fieldName,
			Type:       fieldType,
			NativeType: strings.TrimSpace(p.input[typeStart:typeEnd]),
		})

		p.skipWhitespace()
		if p.peek() == ',' {
			p.pos++
			continue
		}
		if p.peek() == '>' {
			p.pos++
			break
		}
		return nil, fmt.Errorf("expected ',' or '>' in struct at position %d", p.pos)
	}
	return StructType{Fields: fields}, nil
}

func (p *typeParser) skipWhitespace() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}

func (p *typeParser) readIdentifier() string {
	start := p.pos
	for p.pos < len(p.input) && (unicode.IsLetter(rune(p.input[p.pos])) || unicode.IsDigit(rune(p.input[p.pos]))) {
		p.pos++
	}
	return p.input[start:p.pos]
}

func (p *typeParser) readFieldName() string {
	start := p.pos
	for p.pos < len(p.input) && (unicode.IsLetter(rune(p.input[p.pos])) || unicode.IsDigit(rune(p.input[p.pos])) || p.input[p.pos] == '_') {
		p.pos++
	}
	return p.input[start:p.pos]
}

func (p *typeParser) readDigits() string {
	start := p.pos
	for p.pos < len(p.input) && unicode.IsDigit(rune(p.input[p.pos])) {
		p.pos++
	}
	return p.input[start:p.pos]
}

func (p *typeParser) expect(s string) error {
	p.skipWhitespace()
	if !strings.HasPrefix(p.input[p.pos:], s) {
		return fmt.Errorf("expected '%s' at position %d", s, p.pos)
	}
	p.pos += len(s)
	return nil
}

func (p *typeParser) peek() byte {
	if p.pos < len(p.input) {
		return p.input[p.pos]
	}
	return 0
}
