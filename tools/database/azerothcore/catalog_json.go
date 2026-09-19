package azerothcore

import (
	"bytes"
	"cmp"
	"encoding/json"
	"slices"

	"github.com/wowsims/wotlk/sim/core/proto"
	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
)

// MarshalCatalogJSON writes the catalog as protojson laid out like db.json: one item or limit
// group per line, sorted by id, default values left out. The same catalog always gives the same
// bytes.
func MarshalCatalogJSON(catalog *proto.ServerCatalog) ([]byte, error) {
	items := slices.Clone(catalog.Items)
	slices.SortFunc(items, func(a, b *proto.CatalogItem) int { return cmp.Compare(a.Id, b.Id) })
	groups := slices.Clone(catalog.LimitGroups)
	slices.SortFunc(groups, func(a, b *proto.LimitGroup) int { return cmp.Compare(a.Id, b.Id) })

	var buf bytes.Buffer
	date, err := json.Marshal(catalog.Date)
	if err != nil {
		return nil, err
	}
	buf.WriteString("{\n\"date\":")
	buf.Write(date)
	buf.WriteString(",\n")
	if err := writeProtoLines(&buf, "items", items); err != nil {
		return nil, err
	}
	buf.WriteString(",\n")
	if err := writeProtoLines(&buf, "limitGroups", groups); err != nil {
		return nil, err
	}
	buf.WriteString("\n}\n")
	return buf.Bytes(), nil
}

func writeProtoLines[T googleProto.Message](buf *bytes.Buffer, name string, messages []T) error {
	buf.WriteString("\"" + name + "\":[\n")
	for i, m := range messages {
		data, err := protojson.MarshalOptions{UseEnumNumbers: true}.Marshal(m)
		if err != nil {
			return err
		}
		// protojson's spacing isn't stable across runs; Compact is
		if err := json.Compact(buf, data); err != nil {
			return err
		}
		if i < len(messages)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("]")
	return nil
}

// ParseCatalogJSON reads a catalog MarshalCatalogJSON wrote.
func ParseCatalogJSON(data []byte) (*proto.ServerCatalog, error) {
	catalog := &proto.ServerCatalog{}
	if err := protojson.Unmarshal(data, catalog); err != nil {
		return nil, err
	}
	return catalog, nil
}
