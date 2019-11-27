package senml

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"sort"
	"time"

	"github.com/fxamacker/cbor"
)

const (
	xmlns          = "urn:ietf:params:xml:ns:senml"
	second         = int64(time.Second)
	defaultVersion = 10
)

// Format represents SenML message format.
type Format int

// Supported formats are JSON, XML, and CBOR.
const (
	JSON Format = 1 + iota
	XML
	CBOR
)

// Record represents one senML record.
type Record struct {
	XMLName     *bool    `json:"_,omitempty" xml:"senml"`
	BaseName    string   `json:"bn,omitempty" xml:"bn,attr,omitempty" cbor:"-2,keyasint,omitempty"`
	BaseTime    float64  `json:"bt,omitempty" xml:"bt,attr,omitempty" cbor:"-3,keyasint,omitempty"`
	BaseUnit    string   `json:"bu,omitempty" xml:"bu,attr,omitempty" cbor:"-4,keyasint,omitempty"`
	BaseVersion uint     `json:"bver,omitempty" xml:"bver,attr,omitempty" cbor:"-1,keyasint,omitempty"`
	BaseValue   float64  `json:"bv,omitempty" xml:"bv,attr,omitempty" cbor:"-5,keyasint,omitempty"`
	BaseSum     float64  `json:"bs,omitempty" xml:"bs,attr,omitempty" cbor:"-6,keyasint,omitempty"`
	Name        string   `json:"n,omitempty" xml:"n,attr,omitempty" cbor:"0,keyasint,omitempty"`
	Unit        string   `json:"u,omitempty" xml:"u,attr,omitempty" cbor:"1,keyasint,omitempty"`
	Time        float64  `json:"t,omitempty" xml:"t,attr,omitempty" cbor:"6,keyasint,omitempty"`
	UpdateTime  float64  `json:"ut,omitempty" xml:"ut,attr,omitempty" cbor:"7,keyasint,omitempty"`
	Value       *float64 `json:"v,omitempty" xml:"v,attr,omitempty" cbor:"2,keyasint,omitempty"`
	StringValue *string  `json:"vs,omitempty" xml:"vs,attr,omitempty" cbor:"3,keyasint,omitempty"`
	DataValue   *string  `json:"vd,omitempty" xml:"vd,attr,omitempty" cbor:"8,keyasint,omitempty"`
	BoolValue   *bool    `json:"vb,omitempty" xml:"vb,attr,omitempty" cbor:"4,keyasint,omitempty"`
	Sum         *float64 `json:"s,omitempty" xml:"s,attr,omitempty" cbor:"5,keyasint,omitempty"`
}

// Pack consists of SenML records array.
type Pack struct {
	XMLName *bool    `json:"_,omitempty" xml:"sensml"`
	Xmlns   string   `json:"_,omitempty" xml:"xmlns,attr"`
	Records []Record `xml:"senml"`
}

// Implement sort.Interface so that resolved recods can easily be sorted.
func (p *Pack) Len() int {
	return len(p.Records)
}

func (p *Pack) Less(i, j int) bool {
	return p.Records[i].Time < p.Records[j].Time
}

func (p *Pack) Swap(i, j int) {
	p.Records[i], p.Records[j] = p.Records[j], p.Records[i]
}

// Decode takes a SenML message in the given format and parses it and decodes it
// into the returned SenML record.
func Decode(msg []byte, format Format) (Pack, error) {
	var p Pack
	p.Xmlns = xmlns
	switch format {
	case JSON:
		if err := json.Unmarshal(msg, &p.Records); err != nil {
			return Pack{}, err
		}
	case XML:
		if err := xml.Unmarshal(msg, &p); err != nil {
			return Pack{}, err
		}
	case CBOR:
		if err := cbor.Unmarshal(msg, &p.Records); err != nil {
			return Pack{}, err
		}
	}

	return p, Validate(p)
}

// Encode takes a SenML Pack and encodes it using the given format.
func Encode(p Pack, format Format) ([]byte, error) {
	p.Xmlns = xmlns
	switch format {
	case JSON:
		return json.Marshal(p.Records)
	case XML:
		return xml.Marshal(p)
	case CBOR:
		return cbor.Marshal(p.Records, cbor.EncOptions{})
	default:
		return nil, errors.New("unsupported format")
	}
}

// Normalize removes all the base values and expands records values  base items.
// The base fields apply to the entries in the Record and also to all Records after
// it up to, but not including, the next Record that has that same base field.
func Normalize(p Pack) (Pack, error) {
	// Validate ensures that all the BaseVersions are equal.
	if err := Validate(p); err != nil {
		return Pack{}, err
	}
	records := make([]Record, len(p.Records))
	var bname string
	var btime float64
	var bsum float64
	var bunit string

	for i, r := range p.Records {
		if r.BaseTime != 0 {
			btime = r.BaseTime
		}
		if r.BaseSum != 0 {
			bsum = r.BaseSum
		}
		if len(r.BaseUnit) > 0 {
			bunit = r.BaseUnit
		}
		if len(r.BaseName) > 0 {
			bname = r.BaseName
		}
		r.Name = bname + r.Name
		r.Time = btime + r.Time
		if r.Sum != nil {
			*r.Sum = bsum + *r.Sum
		}
		if len(r.Unit) == 0 {
			r.Unit = bunit
		}
		if r.Value != nil && r.BaseValue != 0 {
			*r.Value = r.BaseValue + *r.Value
		}
		// If the version is default, it must not be present in resolved records.
		// Validate method takes care that the version is the same on all the records.
		if r.BaseVersion == defaultVersion {
			r.BaseVersion = 0
		}

		// Remove Base Values to
		r.BaseTime = 0
		r.BaseValue = 0
		r.BaseUnit = ""
		r.BaseName = ""
		r.BaseSum = 0
		records[i] = r
	}
	p.Records = records
	sort.Sort(&p)
	return p, nil
}

// Validate validates SenML records.
func Validate(p Pack) error {
	var bver uint
	for _, r := range p.Records {
		// Check if version is the same for all records.
		if bver == 0 && r.BaseVersion != 0 {
			bver = r.BaseVersion
		}
		if r.BaseVersion != bver {
			return errors.New("unalowed version change")
		}
		name := r.BaseName + r.Name
		if len(name) == 0 {
			return errors.New("empty name")
		}
		l := name[0]
		if l == '-' || l == ':' || l == '.' || l == '/' || l == '_' {
			return errors.New("bad name start")
		}
		for _, l := range name {
			if (l < 'a' || l > 'z') && (l < 'A' || l > 'Z') && (l < '0' || l > '9') && l != '-' && l != ':' && l != '.' && l != '/' && l != '_' {
				return errors.New("bad char in name")
			}
		}
		valueCount := 0
		if r.Value != nil {
			valueCount++
		}
		if r.BoolValue != nil {
			valueCount++
		}
		if r.DataValue != nil {
			valueCount++
		}
		if r.StringValue != nil {
			valueCount++
		}
		if valueCount > 1 {
			return errors.New("too many values")
		}
		if r.Sum != nil {
			valueCount++
		}
		if valueCount < 1 {
			return errors.New("No value or sum")
		}
	}

	return nil
}
