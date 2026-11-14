package ratio_setting

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

// ratioJSONMap keeps a float64 map for display/JSON compatibility and a
// decimal map as the financial authority. JSON ingest parses numbers with
// json.Number so literals such as 0.1 are stored exactly. Legacy float
// values are converted once at the load boundary.
type ratioJSONMap struct {
	floats   *types.RWMap[string, float64]
	decimals *types.RWMap[string, decimal.Decimal]
}

func newRatioJSONMap(floats *types.RWMap[string, float64], decimals *types.RWMap[string, decimal.Decimal]) *ratioJSONMap {
	return &ratioJSONMap{floats: floats, decimals: decimals}
}

func (m *ratioJSONMap) UnmarshalJSON(b []byte) error {
	return m.loadJSON(string(b))
}

func (m *ratioJSONMap) MarshalJSON() ([]byte, error) {
	return m.floats.MarshalJSON()
}

func (m *ratioJSONMap) loadJSON(jsonStr string) error {
	decimals, err := parseDecimalObject(jsonStr)
	if err != nil {
		return err
	}
	m.replaceAll(decimals)
	return nil
}

func (m *ratioJSONMap) replaceAll(decimals map[string]decimal.Decimal) {
	floats := make(map[string]float64, len(decimals))
	for key, value := range decimals {
		floats[key] = value.InexactFloat64()
	}
	m.decimals.ReplaceAll(decimals)
	m.floats.ReplaceAll(floats)
}

func (m *ratioJSONMap) syncFromFloats() {
	all := m.floats.ReadAll()
	decimals := make(map[string]decimal.Decimal, len(all))
	for key, value := range all {
		decimals[key] = DecimalFromFloat(value)
	}
	m.decimals.ReplaceAll(decimals)
}

func (m *ratioJSONMap) getDecimal(name string) (decimal.Decimal, bool) {
	return m.decimals.Get(name)
}

type nestedRatioJSONMap struct {
	floats   *types.RWMap[string, map[string]float64]
	decimals *types.RWMap[string, map[string]decimal.Decimal]
}

func newNestedRatioJSONMap(floats *types.RWMap[string, map[string]float64], decimals *types.RWMap[string, map[string]decimal.Decimal]) *nestedRatioJSONMap {
	return &nestedRatioJSONMap{floats: floats, decimals: decimals}
}

func (m *nestedRatioJSONMap) UnmarshalJSON(b []byte) error {
	return m.loadJSON(string(b))
}

func (m *nestedRatioJSONMap) MarshalJSON() ([]byte, error) {
	return m.floats.MarshalJSON()
}

func (m *nestedRatioJSONMap) loadJSON(jsonStr string) error {
	decimals, err := parseNestedDecimalObject(jsonStr)
	if err != nil {
		return err
	}
	m.replaceAll(decimals)
	return nil
}

func (m *nestedRatioJSONMap) replaceAll(decimals map[string]map[string]decimal.Decimal) {
	floats := make(map[string]map[string]float64, len(decimals))
	for outer, inner := range decimals {
		converted := make(map[string]float64, len(inner))
		for key, value := range inner {
			converted[key] = value.InexactFloat64()
		}
		floats[outer] = converted
	}
	m.decimals.ReplaceAll(decimals)
	m.floats.ReplaceAll(floats)
}

func (m *nestedRatioJSONMap) syncFromFloats() {
	all := m.floats.ReadAll()
	decimals := make(map[string]map[string]decimal.Decimal, len(all))
	for outer, inner := range all {
		converted := make(map[string]decimal.Decimal, len(inner))
		for key, value := range inner {
			converted[key] = DecimalFromFloat(value)
		}
		decimals[outer] = converted
	}
	m.decimals.ReplaceAll(decimals)
}

func (m *nestedRatioJSONMap) getDecimal(userGroup, usingGroup string) (decimal.Decimal, bool) {
	inner, ok := m.decimals.Get(userGroup)
	if !ok {
		return decimal.Zero, false
	}
	ratio, ok := inner[usingGroup]
	return ratio, ok
}

func parseDecimalObject(jsonStr string) (map[string]decimal.Decimal, error) {
	raw, err := decodeJSONObject(jsonStr)
	if err != nil {
		return nil, err
	}
	out := make(map[string]decimal.Decimal, len(raw))
	for key, value := range raw {
		parsed, err := decimalFromJSONValue(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = parsed
	}
	return out, nil
}

func parseNestedDecimalObject(jsonStr string) (map[string]map[string]decimal.Decimal, error) {
	raw, err := decodeJSONObject(jsonStr)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]decimal.Decimal, len(raw))
	for outer, value := range raw {
		innerRaw, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: want a JSON object of numbers", outer)
		}
		inner := make(map[string]decimal.Decimal, len(innerRaw))
		for key, item := range innerRaw {
			parsed, err := decimalFromJSONValue(item)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", outer, key, err)
			}
			inner[key] = parsed
		}
		out[outer] = inner
	}
	return out, nil
}

func decodeJSONObject(jsonStr string) (map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(jsonStr))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("invalid trailing data")
	}
	if raw == nil {
		return map[string]any{}, nil
	}
	return raw, nil
}

func decimalFromJSONValue(value any) (decimal.Decimal, error) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := decimal.NewFromString(string(typed))
		if err != nil {
			return decimal.Zero, err
		}
		return parsed, nil
	case nil:
		return decimal.Zero, fmt.Errorf("numeric value required")
	default:
		return decimal.Zero, fmt.Errorf("numeric value required, got %T", value)
	}
}
