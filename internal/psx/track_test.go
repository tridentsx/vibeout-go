package psx

import (
	"encoding/binary"
	"testing"
)

func TestDecodeTRSCollisionFlags(t *testing.T) {
	data := make([]byte, trackSectionSize)
	binary.BigEndian.PutUint32(data[148:152], 0x00180020)
	sections, err := DecodeTRS(data)
	if err != nil {
		t.Fatal(err)
	}
	if sections[0].CollisionFlags != 0x00180020 {
		t.Fatalf("CollisionFlags = %#x", sections[0].CollisionFlags)
	}
	if sections[0].Flags != 0x0020 {
		t.Fatalf("Flags = %#x", sections[0].Flags)
	}
}
