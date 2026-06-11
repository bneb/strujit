package main

import (
	"testing"
)

func TestParseSchema(t *testing.T) {
	schemaStr := "uint32 symbol, uint64 ts, float64 price, uint32 size"
	fields, sch := parseSchema(schemaStr)

	if len(fields) != 4 {
		t.Errorf("expected 4 fields, got %d", len(fields))
	}

	if sch.StructSize != 32 {
		t.Errorf("expected struct size 32 with padding, got %d", sch.StructSize)
	}

	if fields["symbol"].IRType != "i32" {
		t.Errorf("expected symbol IRType to be i32, got %s", fields["symbol"].IRType)
	}
	if fields["price"].IRType != "double" {
		t.Errorf("expected price IRType to be double, got %s", fields["price"].IRType)
	}
}

func TestApplyPadding(t *testing.T) {
	var defs []string
	// offset 4, align 8 -> needs 4 bytes padding to reach 8
	newOffset := applyPadding(4, 8, &defs)
	if newOffset != 8 {
		t.Errorf("expected new offset 8, got %d", newOffset)
	}
	if len(defs) != 1 || defs[0] != "[4 x i8]" {
		t.Errorf("expected [4 x i8] padding, got %v", defs)
	}
}

func TestResolveTypeProps(t *testing.T) {
	size, align, irType := resolveTypeProps("float64")
	if size != 8 || align != 8 || irType != "double" {
		t.Errorf("float64 mapping failed: %d, %d, %s", size, align, irType)
	}

	size, align, irType = resolveTypeProps("uint8")
	if size != 1 || align != 1 || irType != "i8" {
		t.Errorf("uint8 mapping failed: %d, %d, %s", size, align, irType)
	}
}
