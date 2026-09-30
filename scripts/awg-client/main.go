// Test-only AmneziaWG client. All configuration arrives on stdin, never argv.
package main

import (
	"bufio"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"os"
	"time"

	awgconn "github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"
	"github.com/amnezia-vpn/amneziawg-go/v3/tun/netstack"
)

type command struct{ Op, Config, Address, Network, Destination string }

func main() {
	var dev *device.Device
	var network *netstack.Net
	defer func() {
		if dev != nil {
			dev.Close()
		}
	}()
	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() {
		var c command
		if err := json.Unmarshal(scan.Bytes(), &c); err != nil {
			panic(err)
		}
		result := map[string]any{"ok": true}
		err := func() error {
			switch c.Op {
			case "keys":
				key, err := ecdh.X25519().GenerateKey(rand.Reader)
				if err != nil {
					return err
				}
				result["private"] = base64.StdEncoding.EncodeToString(key.Bytes())
				result["public"] = base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())
			case "setup":
				if dev != nil {
					dev.Close()
				}
				addr, err := netip.ParseAddr(c.Address)
				if err != nil {
					return err
				}
				tun, n, err := netstack.CreateNetTUN([]netip.Addr{addr}, nil, 1280)
				if err != nil {
					return err
				}
				network = n
				dev = device.NewDevice(tun, awgconn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
				if err := dev.IpcSet(c.Config); err != nil {
					return err
				}
				return dev.Up()
			case "probe":
				if network == nil {
					return fmt.Errorf("client is not configured")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				conn, err := network.DialContext(ctx, c.Network, c.Destination)
				if err != nil {
					return err
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				payload := make([]byte, 1024)
				if _, err := rand.Read(payload); err != nil {
					return err
				}
				if _, err := conn.Write(payload); err != nil {
					return err
				}
				response := make([]byte, len(payload))
				if _, err := io.ReadFull(conn, response); err != nil {
					return err
				}
				for i := range payload {
					if payload[i] != response[i] {
						return fmt.Errorf("echo mismatch")
					}
				}
			default:
				return fmt.Errorf("unknown test command")
			}
			return nil
		}()
		if err != nil {
			result = map[string]any{"ok": false, "error": err.Error()}
		}
		_ = json.NewEncoder(os.Stdout).Encode(result)
	}
}
