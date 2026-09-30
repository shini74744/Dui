// Test-only MTProto client: FakeTLS + obfuscated2 + req_pq_multi/resPQ.
// It creates no Telegram account/session and sends no messages.
package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

type input struct {
	Address, Secret string
	Hello           []byte
	DC              int
}

func digest(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(data)
	return h.Sum(nil)
}

func readRecord(c net.Conn) ([]byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(c, header); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint16(header[3:]))
	if n == 0 {
		return nil, fmt.Errorf("empty TLS record")
	}
	raw := make([]byte, 5+n)
	copy(raw, header)
	_, err := io.ReadFull(c, raw[5:])
	return raw, err
}

func run(in input) error {
	secret, err := hex.DecodeString(in.Secret)
	if err != nil || len(secret) < 18 {
		return fmt.Errorf("invalid test credential")
	}
	key := secret[1:17]
	hello := in.Hello
	if len(hello) < 43 {
		return fmt.Errorf("missing TLS ClientHello")
	}
	clear(hello[11:43])
	random := digest(key, hello)
	stamp := make([]byte, 4)
	binary.LittleEndian.PutUint32(stamp, uint32(time.Now().Unix()))
	for i := 0; i < 4; i++ {
		random[28+i] ^= stamp[i]
	}
	copy(hello[11:43], random)
	conn, err := net.DialTimeout("tcp", in.Address, 4*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	if _, err = conn.Write(hello); err != nil {
		return err
	}
	var welcome []byte
	for _, kind := range []byte{22, 20, 23} {
		record, err := readRecord(conn)
		if err != nil {
			return fmt.Errorf("FakeTLS: %w", err)
		}
		if record[0] != kind {
			return fmt.Errorf("unexpected FakeTLS record")
		}
		welcome = append(welcome, record...)
	}
	if len(welcome) < 43 {
		return fmt.Errorf("short welcome")
	}
	got := bytes.Clone(welcome[11:43])
	clear(welcome[11:43])
	if !hmac.Equal(got, digest(key, append(bytes.Clone(random), welcome...))) {
		return fmt.Errorf("server authentication failed")
	}
	if _, err = conn.Write([]byte{20, 3, 3, 0, 1, 1}); err != nil {
		return err
	}
	writeRecord := func(data []byte) error {
		raw := make([]byte, 5+len(data))
		copy(raw, []byte{23, 3, 3})
		binary.BigEndian.PutUint16(raw[3:5], uint16(len(data)))
		copy(raw[5:], data)
		_, err := conn.Write(raw)
		return err
	}
	// Obfuscated2 layout and key mixing follow Telegram's transport spec.
	frame := make([]byte, 64)
	if _, err = rand.Read(frame); err != nil {
		return err
	}
	copy(frame[56:60], []byte{0xdd, 0xdd, 0xdd, 0xdd})
	binary.LittleEndian.PutUint16(frame[60:62], uint16(in.DC))
	makeCipher := func(material []byte) cipher.Stream {
		h := sha256.Sum256(append(bytes.Clone(material[:32]), key...))
		block, _ := aes.NewCipher(h[:])
		return cipher.NewCTR(block, material[32:48])
	}
	tx := makeCipher(frame[8:56])
	reversed := bytes.Clone(frame[8:56])
	for i, j := 0, 47; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	rx := makeCipher(reversed)
	cipherFrame := make([]byte, 64)
	tx.XORKeyStream(cipherFrame, frame)
	copy(cipherFrame[8:56], frame[8:56])
	if err = writeRecord(cipherFrame); err != nil {
		return err
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	msg := make([]byte, 44)
	binary.LittleEndian.PutUint32(msg, 40)
	now := time.Now()
	msgID := (uint64(now.Unix())<<32 | (uint64(now.Nanosecond())<<32)/1000000000) &^ 3
	binary.LittleEndian.PutUint64(msg[12:20], msgID)
	binary.LittleEndian.PutUint32(msg[20:24], 20)
	binary.LittleEndian.PutUint32(msg[24:28], 0xbe7e8ef1)
	copy(msg[28:44], nonce)
	tx.XORKeyStream(msg, msg)
	if err = writeRecord(msg); err != nil {
		return err
	}
	var response []byte
	for len(response) < 4 || len(response) < 4+int(binary.LittleEndian.Uint32(response[:4])) {
		record, err := readRecord(conn)
		if err != nil {
			return fmt.Errorf("MTProto reply: %w", err)
		}
		if record[0] != 23 {
			return fmt.Errorf("unexpected data record")
		}
		data := record[5:]
		rx.XORKeyStream(data, data)
		response = append(response, data...)
		if len(response) >= 4 && binary.LittleEndian.Uint32(response[:4]) > 4096 {
			return fmt.Errorf("invalid MTProto frame length")
		}
	}
	if len(response) < 44 || binary.LittleEndian.Uint32(response[24:28]) != 0x05162463 || !bytes.Equal(response[28:44], nonce) {
		return fmt.Errorf("resPQ constructor or nonce mismatch")
	}
	return nil
}

func main() {
	var in input
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		panic(err)
	}
	result := map[string]any{"ok": true}
	if err := run(in); err != nil {
		result = map[string]any{"ok": false, "error": err.Error()}
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
}
