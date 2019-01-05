package lifxlan

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

// EmptyHardwareVersion is the constant to be compared against
// Device.HardwareVersion().String().
const EmptyHardwareVersion = "(0, 0, 0)"

// RawStateVersionPayload defines the struct to be used for encoding and
// decoding.
//
// https://lan.developer.lifx.com/v2.0/docs/device-messages#section-stateversion-33
type RawStateVersionPayload struct {
	Version RawHardwareVersion
}

// ProductMapKey generates key for ProductMap based on vendor and product ids.
func ProductMapKey(vendor, product uint32) uint64 {
	return uint64(vendor)<<32 + uint64(product)
}

// ParsedHardwareVersion is the parsed hardware version info.
type ParsedHardwareVersion struct {
	ProductName string

	// Features
	Color     bool
	Infrared  bool
	MultiZone bool
	Chain     bool
	// Both values are inclusive.
	MinKelvin uint16
	MaxKelvin uint16

	// Embedded raw info.
	Raw RawHardwareVersion
}

// RawHardwareVersion defines raw version info in message payloads according to:
//
// https://lan.developer.lifx.com/v2.0/docs/device-messages#section-stateversion-33
type RawHardwareVersion struct {
	VendorID        uint32
	ProductID       uint32
	HardwareVersion uint32
}

// ProductMapKey generates key for ProductMap.
func (raw RawHardwareVersion) ProductMapKey() uint64 {
	return ProductMapKey(raw.VendorID, raw.ProductID)
}

// Parse parses the raw hardware version info by looking up ProductMap.
//
// If this hardware version info is not in ProductMap, nil will be returned.
func (raw RawHardwareVersion) Parse() *ParsedHardwareVersion {
	parsed, ok := ProductMap[raw.ProductMapKey()]
	if !ok {
		return nil
	}
	parsed.Raw = raw
	return &parsed
}

func (raw RawHardwareVersion) String() string {
	var sb strings.Builder
	parsed := raw.Parse()
	if parsed != nil {
		sb.WriteString(parsed.ProductName)
	}
	sb.WriteString(
		fmt.Sprintf(
			"(%v, %v, %v)",
			raw.VendorID,
			raw.ProductID,
			raw.HardwareVersion,
		),
	)
	return sb.String()
}

func (d *device) HardwareVersion() *RawHardwareVersion {
	return &d.version
}

func (d *device) GetHardwareVersion(ctx context.Context, conn net.Conn) error {
	select {
	default:
	case <-ctx.Done():
		return ctx.Err()
	}

	if conn == nil {
		newConn, err := d.Dial()
		if err != nil {
			return err
		}
		defer newConn.Close()
		conn = newConn

		select {
		default:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	seq, err := d.Send(
		ctx,
		conn,
		NotTagged,
		0, // flags
		GetVersion,
		nil, // payload
	)
	if err != nil {
		return err
	}

	buf := make([]byte, ResponseReadBufferSize)
	for {
		select {
		default:
		case <-ctx.Done():
			return ctx.Err()
		}

		if err := conn.SetReadDeadline(GetReadDeadline()); err != nil {
			return err
		}

		n, err := conn.Read(buf)
		if err != nil {
			if CheckTimeoutError(err) {
				continue
			}
			return err
		}

		resp, err := ParseResponse(buf[:n])
		if err != nil {
			return err
		}
		if resp.Sequence != seq || resp.Source != d.Source() {
			continue
		}
		if resp.Message != StateVersion {
			continue
		}

		var raw RawStateVersionPayload
		r := bytes.NewReader(resp.Payload)
		if err := binary.Read(r, binary.LittleEndian, &raw); err != nil {
			return err
		}

		d.version = raw.Version
		return nil
	}
}
