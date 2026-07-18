package relaybox

import (
	"bufio"
	"net"
)

// bufferedConn is a net.Conn whose reads are served from a bufio.Reader
// first. It exists because reading the REGISTER/REDEEM header line with a
// bufio.Reader can pull a few extra bytes past the line into the reader's
// internal buffer; once the connection becomes a data line those bytes must
// still be relayed, not dropped. Writes go straight to the underlying
// connection.
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (conn *bufferedConn) Read(buffer []byte) (int, error) {
	return conn.reader.Read(buffer)
}
