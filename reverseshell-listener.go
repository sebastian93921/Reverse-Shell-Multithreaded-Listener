package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ctrlCChan = make(chan os.Signal, 1)
var backgroundCommand = "rev-bg"

func main() {
	fmt.Println("===========================")
	fmt.Println(" Reverse Shell listener")
	fmt.Println("===========================")

	// Keyboard signal notify
	signal.Notify(ctrlCChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	var destinationPort string
	clients := map[int]*Socket{}

	flag.Parse()
	if flag.NFlag() == 0 && flag.NArg() == 0 {
		fmt.Println("Usage: reverseshell-listener <port>")
		os.Exit(1)
	}

	if _, err := strconv.Atoi(flag.Arg(0)); err != nil {
		fmt.Println("[!] Port cannot be empty and not an integer")
		os.Exit(1)
	} else {
		destinationPort = fmt.Sprintf(":%v", flag.Arg(0))
	}

	fmt.Println("[+] Listening on port", destinationPort)
	go connectionThread(destinationPort, clients)

	reader := bufio.NewReader(os.Stdin)
	connectedSession := 1
	for {
		select {
		case <-ctrlCChan:
			fmt.Println("\n[+] Application quit successfully")
			os.Exit(0)
		default:
			if len(clients) > 0 && connectedSession == 0 {
				fmt.Print("listener> ")
				text, _ := reader.ReadString('\n')
				connectedSession = commandHandler(text, clients)
			} else if len(clients) > 0 && connectedSession != 0 {
				if !clients[connectedSession].isClosed {
					clients[connectedSession].interact()
				} else {
					fmt.Println("[-] No matched session or session has been closed")
				}
				connectedSession = 0
			}
			time.Sleep(1 * time.Microsecond)
		}

	}
}

func commandHandler(cmd string, clients map[int]*Socket) int {
	connectedSession := 0

	splitCommand := strings.Split(cmd, " ")
	switch strings.TrimSuffix(splitCommand[0], "\n") {
	case "help":
		fmt.Println("sessions \t- List sessions")
		fmt.Println("session <num> \t- Get into session by ID")
	case "sessions":
		fmt.Println("---------------------->")
		for _, client := range clients {
			fmt.Println(client.status())
		}
		fmt.Println("<----------------------")
	case "session":
		connectedSession, _ = strconv.Atoi(strings.TrimSuffix(splitCommand[1], "\n"))
		if connectedSession > len(clients) {
			fmt.Println("[!] Wrong session selected")
			connectedSession = 0
		}
	}

	return connectedSession
}

func connectionThread(destPort string, clients map[int]*Socket) {
	listener, err := net.Listen("tcp", destPort)
	if err != nil {
		fmt.Println("[-]", err)
	}
	//Assign session ID
	sessionId := 1
	for {
		// Listen for an incoming connection.
		con, err := listener.Accept()
		if err != nil {
			fmt.Println("[-] Error accepting:", err)
		}
		// Handle connections in a new goroutine.
		fmt.Println("[+] Got connection from <", con.RemoteAddr().String(), ">, Session ID:", sessionId)
		socket := &Socket{sessionId: sessionId, con: con}
		clients[sessionId] = socket
		sessionId = sessionId + 1
	}
}

/*
	Socket
*/
type Socket struct {
	sessionId    int
	con          net.Conn
	isBackground bool
	isClosed     bool
}

func (s *Socket) interact() {
	s.isBackground = false

	fmt.Printf("[+] Interact with Session ID: %d \n", s.sessionId)
	fmt.Printf("[!] Type '%s' to background the current session\n", backgroundCommand)
	fmt.Println("[+] Happy cracking!")
	// Mark two signal for informational
	stdoutThread := s.copyFromConnection(s.con, os.Stdout)
	stdinThread := s.readingFromStdin(os.Stdin, s.con)
	select {
	case <-stdoutThread:
		fmt.Println("[-] Remote connection is closed")
	case <-stdinThread:
		// fmt.Println("[-] DEBUG: Terminated by user",stdinThread)
	}
}

func (s *Socket) copyFromConnection(src io.Reader, dst io.Writer) <-chan int {
	buf := make([]byte, 1024)
	syncChannel := make(chan int)
	go func() {
		// Defer handling
		defer func() {
			if con, ok := dst.(net.Conn); ok {
				if s.isClosed {
					con.Close()
					fmt.Println("[-] Connection closed:", con.RemoteAddr())
				}
			}
			// Notify that processing is finished
			syncChannel <- 0
		}()
		for {
			var nBytes int
			var err error
			nBytes, err = src.Read(buf)
			if s.isBackground || s.isClosed {
				break
			}
			if err != nil {
				if err != io.EOF {
					fmt.Println("[!] Read error:", err)
					s.isClosed = true
				}
				break
			}
			_, err = dst.Write(buf[0:nBytes])
			if err != nil {
				fmt.Println("[!] Write error:", err)
				s.isClosed = true
			}
		}
	}()
	return syncChannel
}

func (s *Socket) readingFromStdin(src io.Reader, dst io.Writer) <-chan int {
	buf := make([]byte, 1024)
	syncChannel := make(chan int)
	inputChan := make(chan []byte)

	// Input handler
	// Read on ctrl+c/z and input channel
	go func() {
		defer func() {
			fmt.Println("\n[-] Input handler has closed")
		}()
		for {
			if !s.isClosed {
				var sendErr error
				select {
				case <-ctrlCChan:
					// Ctrl+C handle
					result := prompt(fmt.Sprintf("\n[+] Do you really want to quit session [%d] ?", s.sessionId), inputChan)
					if result {
						s.isClosed = true
						return
					} else {
						_, sendErr = dst.Write([]byte("\\003\n"))
					}
				case buf := <-inputChan:
					// Normal input channel
					_, sendErr = dst.Write(buf)
				}
				if sendErr != nil {
					fmt.Println("\n[!] Write error:", sendErr)
					s.isClosed = true
				}
			}
		}
	}()

	go func() {
		// Defer handling
		defer func() {
			if con, ok := dst.(net.Conn); ok {
				if s.isClosed {
					con.Close()
					fmt.Println("\n[-] Connection killed:", con.RemoteAddr())
				} else {
					fmt.Println("\n[-] Backgrounded session", s.sessionId, ", status:", s.isBackground)
				}
			}

			//Cleanup
			close(inputChan)
			// Notify that processing is finished
			syncChannel <- 0
		}()
		for {
			var nBytes int
			var err error

			nBytes, err = src.Read(buf)
			// Special command
			command := string(buf[0 : nBytes-1])
			if command == backgroundCommand {
				s.isBackground = true
			}

			if s.isBackground || s.isClosed {
				break
			}
			if err != nil {
				if err != io.EOF {
					fmt.Println("[!] Read error:", err)
					s.isClosed = true
				}
				break
			}

			// Send input to the input channel
			inputChan <- buf[0:nBytes]
		}
	}()
	return syncChannel
}

func (s *Socket) status() string {
	return fmt.Sprintf("Session ID: [%d], Connection <%s> Seesion killed [%t]", s.sessionId, s.con.RemoteAddr(), s.isClosed)
}

func prompt(message string, inputChan chan []byte) bool {
	for {
		fmt.Print(message + " (Y/N): ")
		buf := <-inputChan
		input := strings.TrimSuffix(string(buf), "\n")
		input = strings.ToUpper(input)
		if input == "Y" || input == "N" {
			return input == "Y"
		}
	}
}