package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type HTTPRequest struct {
	Method   string
	Path     string
	Protocol string
	Headers  http.Header
	Body     []byte
}

func main() {
	fmt.Println("Logs from your program will appear here!")

	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221")

	}

	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())

		}

		go connectionHandler(conn)
	}
}

func connectionHandler(conn net.Conn) {
	request, err := parseData(conn)

	if err != nil {
		fmt.Println("Error during parsing data: ", err.Error())
		fmt.Println("Quitting!")
		os.Exit(1)
	}

	if request.Method == "GET" {
		if request.Path == "/" {
			httpGetBase(conn)
		} else if strings.HasPrefix(request.Path, "/echo/") {
			httpGetEcho(conn, request.Path, request.Headers)
		} else if request.Path == "/user-agent" {
			httpGetUserAgent(conn, request.Headers.Clone().Get("User-Agent"))
		} else if strings.HasPrefix(request.Path, "/files/") {
			httpGetFiles(conn, request.Path)
		} else {
			fmt.Println("Error invalid GET content")
			http_404(conn)
		}
	} else if request.Method == "POST" {
		if strings.HasPrefix(request.Path, "/files") {
			len, _ := strconv.Atoi(request.Headers.Clone().Get("Content-Length"))
			trimmedSlice := request.Body[:len]
			httpPostFiles(conn, request.Path, trimmedSlice)
		} else {
			fmt.Println("Error invalid POST content")
			http_404(conn)
		}
	}
}

func parseData(conn net.Conn) (HTTPRequest, error) {
	req := make([]byte, 1024)
	_, err := conn.Read(req)
	if err != nil {
		fmt.Println("Error while reading from Client", err.Error())
	}

	reader := bufio.NewReader(strings.NewReader(string(req)))

	requestLine, err := reader.ReadString('\n')
	if err != nil {
		fmt.Println("Error reading request line:", err)
		return HTTPRequest{}, err
	}

	// Parse the request line
	parts := strings.Fields(requestLine)
	if len(parts) < 3 {
		fmt.Println("Invalid request line format")
		return HTTPRequest{}, err
	}

	method := parts[0]
	path := parts[1]
	protocol := parts[2]

	// Read headers until an empty line is encountered
	headers := make(http.Header)
	for {
		line, err := reader.ReadString('\n')
		if err != nil || line == "\r\n" {
			break
		}
		headerParts := strings.SplitN(line, ":", 2)
		if len(headerParts) == 2 {
			key := strings.TrimSpace(headerParts[0])
			value := strings.TrimSpace(headerParts[1])
			headers.Add(key, value)
		}
	}

	// Read the request body
	var requestBody bytes.Buffer
	_, err = reader.WriteTo(&requestBody)
	if err != nil {
		fmt.Println("Error reading request body:", err)
		return HTTPRequest{}, err
	}

	// Print or use the parsed components
	fmt.Println("Method:", method)
	fmt.Println("Path:", path)
	fmt.Println("Protocol:", protocol)
	fmt.Println("Headers:")
	for key, values := range headers {
		for _, value := range values {
			fmt.Printf("%s: %s\n", key, value)
		}
	}
	fmt.Println("Request Body:", requestBody.String())

	return HTTPRequest{
		Method:   method,
		Path:     path,
		Protocol: protocol,
		Headers:  headers,
		Body:     requestBody.Bytes(),
	}, nil
}

func http_404(c net.Conn) {
	_, err := c.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\n"))

	if err != nil {
		fmt.Println("Error while responding 404 for a GET", err.Error())
	}
}

func httpGetBase(conn net.Conn) {
	_, err := conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))

	if err != nil {
		fmt.Println("Error while responding 200 for a GET", err.Error())

		http_404(conn)
	}
}

func httpGetEcho(conn net.Conn, path string, header http.Header) {
	msg := strings.TrimPrefix(path, "/echo/")

	var resp string = ""
	if strings.Contains(header.Clone().Get("Accept-Encoding"), "gzip") {
		var buffer bytes.Buffer
		w := gzip.NewWriter(&buffer)
		w.Write([]byte(msg))
		w.Close()
		content := buffer.String()

		resp = "HTTP/1.1 200 OK\r\nContent-Encoding: gzip \r\nContent-Type: text/plain\r\nContent-Length: " + (fmt.Sprint(len(content))) + "\r\n\r\n" + content
	} else {
		resp = "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: " + (fmt.Sprint(len(msg))) + "\r\n\r\n" + msg
	}

	_, err := conn.Write([]byte(resp))

	if err != nil {
		fmt.Println("Error while responding 200 for a GET", err.Error())
		http_404(conn)
	}
}

func httpGetUserAgent(conn net.Conn, uAgent string) {
	const prefix = "User-Agent: "

	msg := strings.TrimPrefix(uAgent, prefix)
	msg = strings.TrimSuffix(msg, "\r\n")

	var resp string = "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: " + (fmt.Sprint(len(msg))) + "\r\n\r\n" + msg

	_, err := conn.Write([]byte(resp))

	if err != nil {
		fmt.Println("Error while responding 200 for a GET", err.Error())
		http_404(conn)
	}
}

func httpGetFiles(conn net.Conn, path string) {
	msg := strings.TrimPrefix(path, "/files/")
	dir := os.Args[2]
	filePath := dir + msg

	if fileInfo, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Printf("File %s does not exist\n", filePath)
		http_404(conn)
	} else if err != nil {
		fmt.Printf("Error failed to get file info: %s\r\n", err)
		http_404(conn)
	} else {
		fmt.Printf("File %s exists\n", filePath)
		f_size := fileInfo.Size()

		var resp string = "HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: " + (fmt.Sprint(f_size)) + "\r\n\r\n"

		file, err := os.Open(filePath)
		if err != nil {
			fmt.Printf("Error failed to open file: %s\r\n", err)
			http_404(conn)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			resp = resp + scanner.Text()
		}
		fmt.Println(resp)

		if err := scanner.Err(); err != nil {
			fmt.Printf("error reading file: %s\r\n", err)
			http_404(conn)
		}

		_, err = conn.Write([]byte(resp))

		if err != nil {
			fmt.Println("Error while responding 200 for a GET", err.Error())
			http_404(conn)
		}
	}
}

func httpPostFiles(conn net.Conn, path string, b []byte) {
	msg := strings.TrimPrefix(path, "/files")
	dir := os.Args[2]
	fullPath := filepath.Join(dir, msg)

	file, err := os.Create(fullPath)
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer file.Close()

	_, err = file.Write(b)
	if err != nil {
		fmt.Println("Error could not write the file", err.Error())
		http_404(conn)
	} else {
		fmt.Printf("Fille created! %s\r\n", fullPath)
	}

	_, err = conn.Write([]byte("HTTP/1.1 201 Created\r\n\r\n"))
	if err != nil {
		fmt.Println("Error while responding 201 for a POST", err.Error())
		http_404(conn)
	}
}
