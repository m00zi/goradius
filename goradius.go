package goradius

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"strings"
)

const (
	headerEnd           = 20
	authenticatorLength = 16
)

type RadiusServer struct {
	Secret string
	// still debating  how we want to handle the policy...
	// do we have one handler and let the user deal with
	// the flow, or
	// do we do it middleware style were we jump from one
	// function to the next...
	handler    func(req *RadiusPacket, res *RadiusPacket) error // option 1
	middleware []func(*RadiusPacket, *RadiusPacket) error       // option 2
}

func (r *RadiusServer) Handler(f func(*RadiusPacket, *RadiusPacket) error) {

	r.handler = f

}

type RadiusRawAttribute struct {
	TypeValue uint8
	Length    uint8
	Value     []byte
}

type RadiusHeader struct {
	Code          uint8
	Identifier    uint8
	Length        uint16
	Authenticator [authenticatorLength]byte
}

type RadiusPacket struct {
	RadiusHeader
	// Attributes []RadiusRawAttribute
	Attributes map[string][]byte
}

func NewRadiusPacket() *RadiusPacket {
	var p RadiusPacket
	p.Attributes = make(map[string][]byte)
	return &p

}

func (p *RadiusPacket) AddAttribute(attrType string, value []byte) error {

	// val := attributes_to_code[attrType]
	// if val == "" {
	// 	errStr := fmt.Sprintf("Uknown attribute: %v", attrType)
	// 	return errors.New(errStr)
	// }

	p.Attributes[attrType] = value
	// attr := RadiusRawAttribute{}
	// attr.TypeValue = attrType
	// attr.Length = len(value)
	// attr.Value = value

	return nil
}

func (p *RadiusPacket) GetAttribute(attrType string) []byte {
	return p.Attributes[attrType]
}

func (r *RadiusServer) handleConn(conn *net.UDPConn) error {

	bufr := make([]byte, 4096)
	rawMsgSize, addr, err := conn.ReadFromUDP(bufr)
	if err != nil {
		panic(err)
	}

	if rawMsgSize < 20 {
		return errors.New("Message to short.")
	}

	rawMsg := bufr[0:rawMsgSize]
	requestPacket, err := r.parseRADIUSPacket(rawMsg)

	responsePacket := NewRadiusPacket()
	responsePacket.RadiusHeader = requestPacket.RadiusHeader

	// for _, m := range r.middleware {
	// 	e := m(requestPacket, responsePacket)
	// 	if e != nil {
	// 	}
	// }

	r.handler(requestPacket, responsePacket)

	output, err := r.encodeRadiusPacket(responsePacket, "secret")
	if err != nil {
		return err
	}

	bytesWritten, err := conn.WriteToUDP(output, addr)
	if bytesWritten != int(responsePacket.Length) {
		log.Printf("WARNING: Written bytes in UDP socket did not match packet size. Packet: %v Written: %v",
			responsePacket.Length, bytesWritten)
	}

	if err != nil {
		return err
	}

	return nil
}

func (r *RadiusServer) ListenAndServe(addr_str string) error {

	addr, err := net.ResolveUDPAddr("udp", addr_str)
	if err != nil {
		log.Fatalln(err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalln(err)
	}

	for {

		e := r.handleConn(conn)
		if e != nil {
			// ignore errors.
			log.Printf("WARNING: %v", e.Error())
		}

	}

}

func (r *RadiusServer) encodeRadiusPacket(packet *RadiusPacket, secret string) ([]byte, error) {

	newBuf := bytes.NewBuffer([]byte{})
	// TODO
	// This is a dumb implementation.
	// Radius Attributes need to be added :)
	// and packet length re-calculated.
	packet.Length = 20

	binary.Write(newBuf, binary.BigEndian, &packet.RadiusHeader)

	output := newBuf.Bytes()

	// Calculate the md5 with the previous authenticator
	// and the current secret.
	md5c := md5.New()
	md5c.Write(output)
	md5c.Write([]byte(secret))
	sum := md5c.Sum(nil)

	// Add the new authenticator data
	offset := 4
	for _, bval := range sum {
		output[offset] = bval
		offset += 1
	}

	return output, nil

}

func (r *RadiusServer) parseRADIUSPacket(rawMsg []byte) (*RadiusPacket, error) {

	packet := NewRadiusPacket()
	reader := bytes.NewReader(rawMsg)

	err := binary.Read(reader, binary.BigEndian, &packet.RadiusHeader)
	if err != nil {
		return nil, err
	}

	rawAttributesBytes := rawMsg[headerEnd:]

	rawAttributes := r.parseAttributes(rawAttributesBytes, packet.Authenticator)

	for _, attr := range rawAttributes {
		name := code_to_attributes[attr.TypeValue]
		packet.AddAttribute(name, attr.Value)
	}

	// TODO convert rawAttributes to readable attrs

	return packet, nil

}

func (r *RadiusServer) parseAttributes(data []byte, requestAuthenticator [16]byte) []RadiusRawAttribute {

	var attrs []RadiusRawAttribute
	reader := bytes.NewBuffer(data)

	for {

		var e error
		var attr_type uint8
		var length uint8

		attr_type, e = reader.ReadByte()
		if e == io.EOF {
			break
		}

		length, e = reader.ReadByte()
		if e == io.EOF {
			break
		}

		value := reader.Next(int(length) - 2)

		if attr_type == 0 {
			break
		}

		// If there is a password we should decrypt it
		if attr_type == 2 {
			value = r.decryptPassword(value, requestAuthenticator)
		}

		attr := RadiusRawAttribute{
			TypeValue: attr_type,
			Length:    length,
			Value:     value,
		}
		attrs = append(attrs, attr)

	}

	return attrs
}

func (r *RadiusServer) decryptPassword(value []byte, requestAuthenticator [16]byte) []byte {

	// TODO: Allow passwords longer than 16 characters...

	var bufr [16]byte

	S := []byte(r.Secret)
	c := requestAuthenticator[0:16]

	_b := md5.New()
	_b.Write(S)
	_b.Write(c)
	b := _b.Sum(nil)

	for i, p := range value {
		bufr[i] = b[i] ^ p
	}

	s := bufr[:strings.Index(string(bufr[0:16]), "\x00")]

	return s
}