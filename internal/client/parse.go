package client

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/amansk/ubereats-pp-cli/internal/exitcode"
	"github.com/amansk/ubereats-pp-cli/internal/model"
)

// ParsePastOrders extracts orders from a getPastOrdersV1 body.
// Field names are hunches; missing paths yield empty strings / zero money.
func ParsePastOrders(raw []byte) (Page, error) {
	if err := failIfEnvelope(raw, "getPastOrdersV1"); err != nil {
		return Page{}, err
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return Page{}, exitcode.APIf("getPastOrdersV1: not json: %w", err)
	}
	data := mapField(root, "data")
	if data == nil {
		data = asMap(root)
	}

	var orders []model.Order
	if om := mapField(data, "ordersMap"); om != nil {
		for _, v := range om {
			if o, ok := normalizeOrder(v); ok {
				orders = append(orders, o)
			}
		}
	}
	if arr, ok := field(data, "orders").([]any); ok && len(arr) > 0 && len(orders) == 0 {
		for _, v := range arr {
			if o, ok := normalizeOrder(v); ok {
				orders = append(orders, o)
			}
		}
	}

	hasMore := boolish(deep(data, "meta", "hasMore"))
	if !hasMore {
		hasMore = boolish(deep(data, "hasMore"))
	}

	next := firstString(
		deep(data, "lastWorkflowUUID"),
		deep(data, "nextLastWorkflowUUID"),
		deep(data, "nextWorkflowUUID"),
		deep(data, "pagination", "lastWorkflowUUID"),
		deep(data, "pagination", "nextCursor"),
	)
	if next == "" && hasMore && len(orders) > 0 {
		// Reported fallback: oldest uuid in the page.
		oldest := orders[0]
		for _, o := range orders {
			if o.OrderedAt.Before(oldest.OrderedAt) {
				oldest = o
			}
		}
		next = oldest.ID
	}

	return Page{
		Orders:     orders,
		HasMore:    hasMore,
		NextCursor: next,
		Raw:        json.RawMessage(append([]byte(nil), raw...)),
	}, nil
}

func normalizeOrder(v any) (model.Order, bool) {
	m := asMap(v)
	if m == nil {
		return model.Order{}, false
	}
	base := mapField(m, "baseEaterOrder")
	if base == nil {
		base = m
	}

	id := firstString(
		m["workflowUUID"], m["workflowUuid"],
		base["workflowUUID"], base["workflowUuid"],
		base["uuid"], m["uuid"],
	)
	if id == "" {
		return model.Order{}, false
	}

	store := mapField(m, "storeInfo")
	if store == nil {
		store = mapField(base, "storeInfo")
	}

	fare := mapField(m, "fareInfo")
	if fare == nil {
		fare = mapField(base, "fareInfo")
	}

	when := firstTime(
		base["completedAt"], base["lastStateChangeAt"], base["createdAt"],
		m["completedAt"], m["created_at"],
	)

	currency := firstString(base["currencyCode"], fare["currencyCode"], m["currencyCode"])
	total := moneyCents(fare["totalPrice"])
	if total == 0 {
		total = moneyCents(base["totalPrice"])
	}

	o := model.Order{
		ID:             id,
		WorkflowUUID:   firstString(m["workflowUUID"], base["workflowUUID"], base["uuid"]),
		RestaurantUUID: firstString(store["uuid"], store["id"]),
		RestaurantName: firstString(store["title"], store["name"]),
		Currency:       currency,
		TotalCents:     total,
		OrderedAt:      when,
		Status:         firstString(m["interactionType"], base["currentState"], base["status"]),
		Items:          extractItems(m, base),
	}
	if o.RestaurantName == "" {
		o.RestaurantName = "Unknown"
	}
	return o, true
}

func extractItems(order, base map[string]any) []model.Item {
	candidates := []any{
		deep(base, "shoppingCart", "items"),
		deep(order, "shoppingCart", "items"),
		deep(base, "items"),
		deep(order, "items"),
		deep(order, "cart", "items"),
	}
	var rawItems []any
	for _, c := range candidates {
		if arr, ok := c.([]any); ok && len(arr) > 0 {
			rawItems = arr
			break
		}
	}
	out := make([]model.Item, 0, len(rawItems))
	for _, v := range rawItems {
		item, ok := normalizeItem(v)
		if ok {
			out = append(out, item)
		}
	}
	return out
}

func normalizeItem(v any) (model.Item, bool) {
	m := asMap(v)
	if m == nil {
		return model.Item{}, false
	}
	title := firstString(m["title"], m["name"], m["itemTitle"], m["titleTranslation"])
	if title == "" {
		if nested := asMap(m["catalogItem"]); nested != nil {
			title = firstString(nested["title"], nested["name"])
		}
	}
	if title == "" {
		return model.Item{}, false
	}
	qty := intish(m["quantity"])
	if qty == 0 {
		qty = intish(m["qty"])
	}
	if qty == 0 {
		qty = 1
	}
	price := moneyCents(m["price"])
	if price == 0 {
		if pm := asMap(m["price"]); pm != nil {
			price = moneyCents(firstNonNil(pm["amount"], pm["unitPrice"], pm["totalPrice"], pm["base_unit_price"]))
		}
	}
	if price == 0 {
		price = moneyCents(m["unitPrice"])
	}
	return model.Item{
		UUID:      firstString(m["uuid"], m["id"], m["catalogItemUuid"]),
		Title:     title,
		Quantity:  qty,
		UnitCents: price,
	}, true
}

func asMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func mapField(v any, key string) map[string]any {
	return asMap(field(v, key))
}

func field(v any, key string) any {
	m := asMap(v)
	if m == nil {
		return nil
	}
	return m[key]
}

func deep(v any, keys ...string) any {
	cur := v
	for _, k := range keys {
		cur = field(cur, k)
		if cur == nil {
			return nil
		}
	}
	return cur
}

func firstString(vals ...any) string {
	for _, v := range vals {
		s := stringify(v)
		if s != "" {
			return s
		}
	}
	return ""
}

func firstNonNil(vals ...any) any {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case json.Number:
		return t.String()
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}

func boolish(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	case float64:
		return t != 0
	default:
		return false
	}
}

func intish(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(t)
		return n
	case int:
		return t
	case int64:
		return int(t)
	default:
		return 0
	}
}

// moneyCents treats reported integers as cents (see PLAN.md).
func moneyCents(v any) int64 {
	switch t := v.(type) {
	case nil:
		return 0
	case float64:
		if t != float64(int64(t)) {
			return int64(t * 100)
		}
		return int64(t)
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return int64(f * 100)
		}
	case string:
		s := strings.TrimSpace(t)
		s = strings.TrimPrefix(s, "$")
		if s == "" {
			return 0
		}
		if strings.Contains(s, ".") {
			f, err := strconv.ParseFloat(s, 64)
			if err == nil {
				return int64(f * 100)
			}
		}
		n, _ := strconv.ParseInt(s, 10, 64)
		return n
	case map[string]any:
		return moneyCents(firstNonNil(t["amount"], t["totalPrice"], t["unitPrice"]))
	}
	return 0
}

func firstTime(vals ...any) time.Time {
	for _, v := range vals {
		t, ok := parseTime(v)
		if ok {
			return t
		}
	}
	return time.Time{}
}

func parseTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return time.Time{}, false
		}
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05.000Z0700",
			"2006-01-02T15:04:05Z07:00",
			"2006-01-02 15:04:05",
			"2006-01-02",
		}
		for _, l := range layouts {
			if tm, err := time.Parse(l, s); err == nil {
				return tm.UTC(), true
			}
		}
		// millis as string
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return epoch(n), true
		}
	case float64:
		return epoch(int64(t)), true
	case json.Number:
		n, err := t.Int64()
		if err == nil {
			return epoch(n), true
		}
	}
	return time.Time{}, false
}

func epoch(n int64) time.Time {
	if n <= 0 {
		return time.Time{}
	}
	// seconds vs millis vs micros
	if n > 1e14 {
		return time.UnixMicro(n).UTC()
	}
	if n > 1e11 {
		return time.UnixMilli(n).UTC()
	}
	return time.Unix(n, 0).UTC()
}
