package main

import (
	"fmt"
	"bufio"
	"net"
	"net/textproto"
	"os"
	"os/signal"
	"syscall"
	"strings"
	"regexp"
	"net/http"
	"net/url"
	"log"
	"encoding/json"
	"github.com/kennygrant/sanitize"
)

type Bot struct {
	server 		string
	port 			string
	nick			string
	channels 	[]string
	conn  		net.Conn	
}

var (
	lineRegexp = regexp.MustCompile(`^(?::(\S+) )?(\S+)(?: (.+?))?(?: :(.+))?$`)
)

func NewBot(server string, port string, nick string, channels []string) *Bot {
	return &Bot {
		server:	server,
		port:	port,
		nick:	nick,
		channels: channels,
		conn:	nil,
	}
}

//return error
func (bot *Bot) Connect() {
	//fixme: hangs script on server connection refusal
	fmt.Printf("Connecting...\n")
	host := bot.server + ":" + bot.port
	if s, err := net.Dial("tcp", host); err == nil {
		bot.conn = s
		fmt.Printf("Connected to: %s\n", bot.server)

		//send connection details to irc server connection (bot.conn)
		fmt.Fprintf(bot.conn, "USER %s 8 * :%s\r\n", bot.nick, bot.nick)
		fmt.Fprintf(bot.conn, "NICK %s\r\n", bot.nick)
		
		for _, channel := range bot.channels {
			fmt.Printf("Joining channel: %s\n", channel)
			fmt.Fprintf(bot.conn, "JOIN %s\r\n", channel)
		}

	} else {
		log.Println("Connect Failed.")
		log.Println(err)
	}
}

func(bot *Bot) sendCommand(command string, commandArgs string, channel string, users string) {
	fmt.Println(command + " " + channel + " " + commandArgs + " " + users)
	fmt.Fprintf(bot.conn, command + " " + channel + " " + commandArgs + " " + users+"\r\n")
}

//bot output to channel 
func (bot *Bot) Message(message string, channel string) {
	if message == "" {
		return
	}
	fmt.Fprintf(bot.conn, "PRIVMSG "+channel+" :"+message+"\r\n")
}

//allows you to issue commands and chat from the console
func (bot *Bot) ConsoleInput() {
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		bot.Message(line, bot.channels[0])
	}
}

func (bot *Bot) RawInput() {
	//instantiate new buffered reader out of the network connection
	reader := bufio.NewReader(bot.conn)
	//instantiate a new textproto reader that reads requests/responses from a "text protocol" network connection
	tp := textproto.NewReader(reader)
	//run the loop
	for {
		line, err := tp.ReadLine()
		if err != nil {
			break
		}

		//goroutine to parse line and do something with it
		go bot.ParseLine(line)
	}
}

func (bot *Bot) ParseLine(line string) {
	//echo line 
	fmt.Println(line)

	//split into parts using regexp
	parts := lineRegexp.FindStringSubmatch(line)
	if parts == nil {
		return
	}

	//send line data into a channel, have functions read the channel and do things

	//respond to pings
	if parts[2] == "PING" {
		fmt.Fprintf(bot.conn, "PONG %s\r\n", parts[3])
		log.Printf("PONG %s\r\n", parts[3])
		return
	}

	identity := parts[1]
	info := parts[2]
	channel := parts[3] //maybe?
	message := parts[4]
	nickname := strings.Split(identity, "!")[0]

	//is command
	if strings.HasPrefix(message, ".") {
		command := strings.Fields(message)
		args := strings.TrimPrefix(message, command[0])
		cleanargs := strings.TrimSpace(args)
		query := url.QueryEscape(cleanargs)

		switch command[0] {
			case ".g":
				bot.google(query, channel)
			default:
				bot.help(query, channel)		
		}
	} else if strings.HasPrefix(info, "JOIN") && nickname == "Pent" {
		bot.sendCommand("MODE", "+o", channel, nickname)
	} else {
		if strings.Contains(message, "hi "+bot.nick) {
			bot.Message("Hi there "+nickname, channel)
		} else { 
			bot.chatter(message, channel)
		}
	}
}


func (bot *Bot) help(query, channel string) {
}

func (bot *Bot) google(query, channel string) {
		r, err := http.Get("http://ajax.googleapis.com/ajax/services/search/web?v=1.0&rsz=1&q="+query)
		defer r.Body.Close()

		if err != nil {
			log.Println(err)
		}
		if r.StatusCode != http.StatusOK {
			log.Println(r.Status)
		}

		//create a custom struct for the json response
		//somehow Go magically transplants the response data into this
		var google struct {
			ResponseData struct {
				Results []struct {
					TitleNoFormatting string
					Content string
					URL string
				}
			}
		}

		//parse response body json to Go
		dec := json.NewDecoder(r.Body)
		dec.Decode(&google)

		//output results to channel
		for _, item := range google.ResponseData.Results {
			//fixme: sending commands
			var content = sanitize.Accents(sanitize.HTML(item.Content))
			bot.Message(item.TitleNoFormatting+" "+item.URL+" "+content, channel)
		}
}

func (bot *Bot) chatter(message, channel string) {
}

//capitalized variable names will export the struct
var settings struct {
    Server string `json:"server"`
    Port string `json:"port"`
    Nickname string `json:"nickname"`
    Channels []string `json:"channels"`
}

func main() {
	//voodoo to allow quitting the app
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	//load configuration
	file, err := os.Open("config.json")
	if err != nil {
		return
	}

	jsonParser := json.NewDecoder(file)
	//assign parsed json file to settings struct
	if err = jsonParser.Decode(&settings); err != nil {
		return
	}

	file.Close()

	//instantiate ircbot
	ircbot := NewBot(settings.Server, settings.Port, settings.Nickname, settings.Channels)
	//call Dial() on the net lib to connect to irc server, and more
	ircbot.Connect()
	//goroutine to run a console input for the bot
	go ircbot.ConsoleInput()
	//goroutine to read lines from irc connection 
	go ircbot.RawInput()
	//push this call into a list, that list is executed ater the surrounding function returns (program exits)
	defer ircbot.conn.Close()

	//hold main running until quit
	<-sigChan
}


