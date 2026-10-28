//go:build stubproto

package ingestprotogen

import (
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Envelope struct {
	SchemaVersion uint32
	Kind          string
	Body          []byte
}

func (*Envelope) ProtoReflect() protoreflect.Message {
	return nil
}

type LightReplay struct {
	Id          string
	StartTs     uint32
	LevelPath   string
	MissionPath string
	Players     []*Player
}

type Player struct {
	Uid  string
	Team uint32
}

type Replay struct {
	Light     *LightReplay
	PosQuant  uint32
	Kills     []*Kill
	SpawnSide []uint32
}

func (*Replay) ProtoReflect() protoreflect.Message {
	return nil
}

type Kill struct {
	OffenderUid      string
	OffenderUnitId   string
	OffenderUnitName string
	OffenderPos      []int32
	UsedWeaponId     string
	UsedWeaponName   string
	OffendedUid      string
	OffendedUnitId   string
	OffendedUnitName string
	OffendedPos      []int32
	Crashed          bool
	Time             uint32
	WeaponType       string
}
