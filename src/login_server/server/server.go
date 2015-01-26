/*
* Archon Login Server
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
*
* Starting point for the login server. Initializes the configuration package and takes care of
* launching the LOGIN and CHARACTER servers. Also provides top-level functions and other code
* shared between the two (found in login.go and character.go).
 */
package server

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"libarchon/encryption"
	"libarchon/util"
	"net"
	"os"
	"sync"
)

var archonDb *sql.DB

// Struct for holding client-specific data.
type LoginClient struct {
	conn   *net.TCPConn
	ipAddr string

	clientCrypt *encryption.PSOCrypt
	serverCrypt *encryption.PSOCrypt

	recvData   []byte
	recvSize   int
	packetSize uint16
}

func (lc LoginClient) Connection() *net.TCPConn { return lc.conn }
func (lc LoginClient) IPAddr() string           { return lc.ipAddr }

// Create and initialize a new struct to hold client information.
func NewClient(conn *net.TCPConn) (*LoginClient, error) {
	client := new(LoginClient)
	client.conn = conn
	client.ipAddr = conn.RemoteAddr().String()

	client.clientCrypt = encryption.NewCrypt()
	client.serverCrypt = encryption.NewCrypt()
	client.clientCrypt.CreateKeys()
	client.serverCrypt.CreateKeys()

	client.recvData = make([]byte, 1024)

	var err error = nil
	if SendWelcome(client) != 0 {
		err = util.ServerError{Message: "Error sending welcome packet to: " + client.ipAddr}
		client = nil
	}
	return client, err
}

func Start() {
	config := GetConfig()
	// Initialize our config singleton from one of two expected file locations.
	fmt.Printf("Loading config file %v...", loginConfigFile)
	err := config.InitFromFile(loginConfigFile)
	if err != nil {
		path := util.ServerConfigDir + "/" + loginConfigFile
		fmt.Printf("Failed.\nLoading config from %v...", path)
		err = config.InitFromFile(path)
		if err != nil {
			fmt.Println("Failed.\nPlease check that one of these files exists and restart the server.")
			os.Exit(-1)
		}
	}
	// TODO: Validate that the configuration struct was populated.
	fmt.Printf("Done.\n--Configuration Parameters--\n%v\n\n", config.String())

	// Initialize the database.
	dbName := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", config.DBUsername,
		config.DBPassword, config.DBHost, config.DBPort, config.DBName)
	fmt.Printf("Connecting to MySQL database...")
	archonDb, err := sql.Open("mysql", dbName)
	if err != nil || archonDb.Ping() != nil {
		fmt.Println("Failed.\nPlease make sure the database connection parameters are correct.")
		os.Exit(-1)
	}
	fmt.Println("Done.")
	defer archonDb.Close()

	// Create a WaitGroup so that main won't exit until the server threads have exited.
	var wg sync.WaitGroup
	wg.Add(2)
	go StartLogin(&wg)
	go StartCharacter(&wg)
	wg.Wait()
}
