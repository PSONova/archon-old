/*
* Archon PSO Server
* Copyright (C) 2014 Andrew Rodman
*
* This program is free software: you can redistribute it and/or modify
* it under the terms of the GNU General Public License as published by
* the Free Software Foundation, either version 3 of the License, or
* (at your option) any later version.
*
* This program is distributed in the hope that it will be useful,
* but WITHOUT ANY WARRANTY; without even the implied warranty of
* MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
* GNU General Public License for more details.
*
* You should have received a copy of the GNU General Public License
* along with this program.  If not, see <http://www.gnu.org/licenses/>.
* ---------------------------------------------------------------------
* Packet types, defintitions, and sending functions.
 */
package main

import (
	"errors"
	"fmt"
	"github.com/dcrodman/archon/util"
	"syscall"
	"time"
)

const (
	// Copyright messages the client expects.
	patchCopyright = "Patch Server. Copyright SonicTeam, LTD. 2001"
	loginCopyright = "Phantasy Star Online Blue Burst Game Server. Copyright 1999-2004 SONICTEAM."
	// Format for the timestamp sent to the client.
	timeFmt = "2006:01:02: 15:05:05"
)

var (
	patchCopyrightBytes []byte
	loginCopyrightBytes []byte
	serverName          = util.ConvertToUtf16("Archon")
)

// Send the packet serialized (or otherwise contained) in pkt to a client.
// Note: Packets sent to BB Clients must have a length divisible by 8.
func sendPacket(c *Client, pkt []byte, length uint16) int {
	if err := c.Send(pkt[:length]); err != nil {
		log.Info("Error sending to client %v: %s", c.IPAddr(), err.Error())
		return -1
	}
	return 0
}

// Send data to client after padding it to a length disible by 8 and
// encrypting it with the client's server ciper.
func sendEncrypted(c *Client, data []byte, length uint16) int {
	data, length = fixLength(data, length, c.hdrSize)
	if config.DebugMode {
		util.PrintPayload(data, int(length))
		fmt.Println()
	}
	c.Encrypt(data, uint32(length))
	return sendPacket(c, data, length)
}

// Pad the length of a packet to a multiple of 8 and set the first two
// bytes of the header.
func fixLength(data []byte, length uint16, hdrSize uint16) ([]byte, uint16) {
	for length%hdrSize != 0 {
		length++
		data = append(data, 0)
	}
	data[0] = byte(length & 0xFF)
	data[1] = byte((length & 0xFF00) >> 8)
	return data, length
}

// Send a simple 4-byte header packet.
func (client *Client) sendPCHeader(pktType uint16) int {
	pkt := &PCHeader{Type: pktType, Size: 0x04}
	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		util.PrintPayload(data, size)
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the welcome packet to a client with the copyright message and encryption vectors.
func (client *Client) SendPCWelcome() int {
	pkt := new(PatchWelcomePkt)
	pkt.Header.Type = PatchWelcomeType
	pkt.Header.Size = 0x4C
	copy(pkt.Copyright[:], patchCopyrightBytes)
	copy(pkt.ClientVector[:], client.ClientVector())
	copy(pkt.ServerVector[:], client.ServerVector())

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Welcome Packet")
		util.PrintPayload(data, size)
		fmt.Println()
	}
	return sendPacket(client, data, uint16(size))
}

func (client *Client) SendWelcomeAck() int {
	pkt := &PCHeader{
		Size: 0x04,
		Type: PatchLoginType, // treated as an ack
	}
	data, _ := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Welcome Ack")
	}
	return sendEncrypted(client, data, 0x0004)
}

// Message displayed on the patch download screen.
func (client *Client) SendWelcomeMessage() int {
	pkt := new(PatchWelcomeMessage)
	pkt.Header = PCHeader{Size: PCHeaderSize + config.MessageSize, Type: PatchMessageType}
	pkt.Message = config.MessageBytes

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Welcome Message")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the redirect packet, providing the IP and port of the next server.
func (client *Client) SendPatchRedirect(port uint16, ipAddr [4]byte) int {
	pkt := new(PatchRedirectPacket)
	pkt.Header.Type = PatchRedirectType
	copy(pkt.IPAddr[:], ipAddr[:])
	pkt.Port = port

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Patch Redirect")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Acknowledgement sent after the DATA connection handshake.
func (client *Client) SendDataAck() int {
	if config.DebugMode {
		fmt.Println("Sending Data Ack")
	}
	return client.sendPCHeader(PatchDataAckType)
}

// Tell the client to change to one directory above.
func (client *Client) SendDirAbove() int {
	if config.DebugMode {
		fmt.Println("Sending Dir Above")
	}
	return client.sendPCHeader(PatchDirAboveType)
}

// Tell the client to change to some directory within its file tree.
func (client *Client) SendChangeDir(dir string) int {
	pkt := new(ChangeDirPacket)
	pkt.Header.Type = PatchChangeDirType
	copy(pkt.Dirname[:], dir)

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Change Directory")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Tell the client to check a file in its current working directory.
func (client *Client) SendCheckFile(index uint32, filename string) int {
	pkt := new(CheckFilePacket)
	pkt.Header.Type = PatchCheckFileType
	pkt.PatchId = index
	copy(pkt.Filename[:], filename)

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Check File")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Inform the client that we've finished sending the patch list.
func (client *Client) SendFileListDone() int {
	if config.DebugMode {
		fmt.Println("Sending List Done")
	}
	return client.sendPCHeader(PatchFileListDoneType)
}

// Send the total number and cumulative size of files that need updating.
func (client *Client) SendUpdateFiles(num, totalSize uint32) int {
	pkt := new(UpdateFilesPacket)
	pkt.Header.Type = PatchUpdateFilesType
	pkt.NumFiles = num
	pkt.TotalSize = totalSize

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Update Files")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the header for a file we're about to update.
func (client *Client) SendFileHeader(patch *PatchEntry) int {
	pkt := new(FileHeaderPacket)
	pkt.Header.Type = PatchFileHeaderType
	pkt.FileSize = patch.fileSize
	copy(pkt.Filename[:], patch.filename)

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending File Header")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send a chunk of file data.
func (client *Client) SendFileChunk(chunk, chksm, chunkSize uint32, fdata []byte) int {
	if chunkSize > MaxFileChunkSize {
		log.Error("Attempted to send %v byte chunk; max is %v",
			string(chunkSize), string(MaxFileChunkSize))
		panic(errors.New("File chunk size exceeds maximum"))
	}
	pkt := &FileChunkPacket{
		Header:   PCHeader{Type: PatchFileChunkType},
		Chunk:    chunk,
		Checksum: chksm,
		Size:     chunkSize,
		Data:     fdata[:chunkSize],
	}

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending File Chunk")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Finished sending a particular file.
func (client *Client) SendFileComplete() int {
	if config.DebugMode {
		fmt.Println("Sending File Complete")
	}
	return client.sendPCHeader(PatchFileCompleteType)
}

// We've finished updating files.
func (client *Client) SendUpdateComplete() int {
	if config.DebugMode {
		fmt.Println("Sending File Update Done")
	}
	return client.sendPCHeader(PatchUpdateCompleteType)
}

// Send the welcome packet to a client with the copyright message and encryption vectors.
func (client *Client) SendWelcome() int {
	pkt := new(WelcomePkt)
	pkt.Header.Type = LoginWelcomeType
	pkt.Header.Size = 0xC8
	copy(pkt.Copyright[:], loginCopyrightBytes)
	copy(pkt.ClientVector[:], client.ClientVector())
	copy(pkt.ServerVector[:], client.ServerVector())

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Welcome Packet")
		util.PrintPayload(data, size)
		fmt.Println()
	}
	return sendPacket(client, data, uint16(size))
}

// Send the security initialization packet with information about the user's
// authentication status.
func (client *Client) SendSecurity(errorCode BBLoginError,
	guildcard uint32, teamId uint32) int {

	// Constants set according to how Newserv does it.
	pkt := &SecurityPacket{
		Header:       BBHeader{Type: LoginSecurityType},
		ErrorCode:    uint32(errorCode),
		PlayerTag:    0x00010000,
		Guildcard:    guildcard,
		TeamId:       teamId,
		Config:       &client.config,
		Capabilities: 0x00000102,
	}

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Security Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the redirect packet, providing the IP and port of the next server.
func (client *Client) SendRedirect(port uint16, ipAddr [4]byte) int {
	pkt := new(RedirectPacket)
	pkt.Header.Type = RedirectType
	copy(pkt.IPAddr[:], ipAddr[:])
	pkt.Port = port

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Redirect Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the client's configuration options. keyConfig should be 420 bytes long and either
// point to the default keys array or loaded from the database.
func (client *Client) SendOptions(keyConfig []byte) int {
	if len(keyConfig) != 420 {
		panic("Received keyConfig of length " + string(len(keyConfig)) + "; should be 420")
	}
	pkt := new(OptionsPacket)
	pkt.Header.Type = LoginOptionsType

	pkt.PlayerKeyConfig.Guildcard = client.guildcard
	copy(pkt.PlayerKeyConfig.KeyConfig[:], keyConfig[:0x16C])
	copy(pkt.PlayerKeyConfig.JoystickConfig[:], keyConfig[0x16C:])

	// Sylverant sets these to enable all team rewards? Not sure what this means yet.
	pkt.PlayerKeyConfig.TeamRewards[0] = 0xFFFFFFFF
	pkt.PlayerKeyConfig.TeamRewards[1] = 0xFFFFFFFF

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Key Config Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the character acknowledgement packet. 0 indicates a creation ack, 1 is
// ack'ing a selected character, and 2 indicates that a character doesn't exist
// in the slot requested via preview request.
func (client *Client) SendCharacterAck(slotNum uint32, flag uint32) int {
	pkt := &CharAckPacket{
		Header: BBHeader{Type: LoginCharAckType},
		Slot:   slotNum,
		Flag:   flag,
	}
	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Character Ack Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the preview packet containing basic details about a character in
// the selected slot.
func (client *Client) SendCharacterPreview(charPreview *CharacterPreview) int {
	pkt := &CharPreviewPacket{
		Header:    BBHeader{Type: LoginCharPreviewType},
		Slot:      0,
		Character: charPreview,
	}
	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Character Preview Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Acknowledge the checksum the client sent us. We don't actually do
// anything with it but the client won't proceed otherwise.
func (client *Client) SendChecksumAck(ack uint32) int {
	pkt := new(ChecksumAckPacket)
	pkt.Header.Type = LoginChecksumAckType
	pkt.Ack = ack

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Checksum Ack Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the guildcard chunk header.
func (client *Client) SendGuildcardHeader(checksum uint32, dataLen uint16) int {
	pkt := &GuildcardHeaderPacket{
		Header:   BBHeader{Type: LoginGuildcardHeaderType},
		Unknown:  0x00000001,
		Length:   dataLen,
		Checksum: checksum,
	}
	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Guildcard Header Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the next chunk of guildcard data.
func (client *Client) SendGuildcardChunk(chunkNum uint32) int {
	pkt := new(GuildcardChunkPacket)
	pkt.Header.Type = LoginGuildcardChunkType
	pkt.Chunk = chunkNum

	// The client will only accept 0x6800 bytes of a chunk per packet.
	offset := uint16(chunkNum) * MaxChunkSize
	remaining := client.gcDataSize - offset
	if remaining > MaxChunkSize {
		pkt.Data = client.gcData[offset : offset+MaxChunkSize]
	} else {
		pkt.Data = client.gcData[offset:]
	}

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Guildcard Chunk Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the header for the parameter files we're about to start sending.
func (client *Client) SendParameterHeader(numEntries uint32, entries []byte) int {
	pkt := &ParameterHeaderPacket{
		Header:  BBHeader{Type: LoginParameterHeaderType, Flags: numEntries},
		Entries: entries,
	}
	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Parameter Header Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Index into chunkData and send the specified chunk of parameter data.
func (client *Client) SendParameterChunk(chunkData []byte, chunk uint32) int {
	pkt := &ParameterChunkPacket{
		Header: BBHeader{Type: LoginParameterChunkType},
		Chunk:  chunk,
		Data:   chunkData,
	}
	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Parameter Chunk Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send an error message to the client, usually used before disconnecting.
func (client *Client) SendClientMessage(message string) int {
	pkt := &LoginClientMessagePacket{
		Header: BBHeader{Type: LoginClientMessageType},
		// English? Tethealla sets this.
		Language: 0x00450009,
		Message:  util.ConvertToUtf16(message),
	}
	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Client Message Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send a timestamp packet in order to indicate the server's current time.
func (client *Client) SendTimestamp() int {
	pkt := new(TimestampPacket)
	pkt.Header.Type = LoginTimestampType

	var tv syscall.Timeval
	syscall.Gettimeofday(&tv)
	t := time.Now().Format(timeFmt)
	stamp := fmt.Sprintf("%s.%03d", t, uint64(tv.Usec/1000))
	copy(pkt.Timestamp[:], stamp)

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Timestamp Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the menu items for the ship select screen.
func (client *Client) SendShipList(ships []Ship) int {
	pkt := &ShipListPacket{
		Header:      BBHeader{Type: LoginShipListType, Flags: 0x01},
		Unknown:     0x02,
		Unknown2:    0xFFFFFFF4,
		Unknown3:    0x04,
		ShipEntries: make([]ShipMenuEntry, len(ships)),
	}
	copy(pkt.ServerName[:], serverName)

	// TODO: Will eventually need a mutex for read.
	for i, ship := range ships {
		item := &pkt.ShipEntries[i]
		item.MenuId = ShipSelectionMenuId
		item.ShipId = ship.id
		copy(item.Shipname[:], util.ConvertToUtf16(string(ship.name[:])))
	}

	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Ship List Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send whatever scrolling message was set in the config file and
// converted to UTF-16LE when the server started up.
func (client *Client) SendScrollMessage() int {
	pkt := &ScrollMessagePacket{
		Header:  BBHeader{Type: LoginScrollMessageType},
		Message: config.ScrollMessageBytes(),
	}
	data, size := util.BytesFromStruct(pkt)
	// The end of the message appears to be garbled unless
	// there is a block of extra bytes on the end; add an extra
	// and let fixLength add the rest.
	data = append(data, 0x00)
	size += 1
	if config.DebugMode {
		fmt.Println("Sending Scroll Message Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the client the block list on the selection screen.
func (client *Client) SendBlockList(pkt *BlockListPacket) int {
	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Block Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

// Send the client the lobby list on the selection screen.
func (client *Client) SendLobbyList(pkt *LobbyListPacket) int {
	data, size := util.BytesFromStruct(pkt)
	if config.DebugMode {
		fmt.Println("Sending Lobby List Packet")
	}
	return sendEncrypted(client, data, uint16(size))
}

func init() {
	patchCopyrightBytes = []byte(patchCopyright)
	loginCopyrightBytes = []byte(loginCopyright)
}
