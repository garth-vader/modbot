package main

import (
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/MemeLabs/dggchat"
)

var (
	debuglogger  = log.New(os.Stdout, "[d] ", log.Ldate|log.Ltime|log.Lshortfile)
	authCookie   string
	chatPath     string
	chatURL      string
	backendURL   string
	logFileName  string
	commandJSON  string
	atAdminToken string
	logOnly      bool
	logFile      *os.File
)

const (
	websiteURL   = "strims.gg"
	pollTime     = time.Second * 2
	ominousEmote = "BOGGED"
)

func main() {
	flag.StringVar(&authCookie, "cookie", "", "Cookie used for chat authentication and API access")
	flag.StringVar(&chatPath, "path", "", "path to chat-gui")
	flag.StringVar(&chatURL, "chat", "wss://chat.strims.gg/ws", "ws(s)-url for chat")
	flag.StringVar(&backendURL, "api", "https://strims.gg/api", "basic backend api path")
	flag.StringVar(&logFileName, "log", "/tmp/chatlog/chatlog.log", "file to write messages to")
	flag.StringVar(&commandJSON, "commands", "commands.json", "static commands file")
	flag.StringVar(&atAdminToken, "attoken", "", "angelthump admin token (optional)")
	flag.BoolVar(&logOnly, "logonly", false, "only 'reply' to logfile, not chat (for debugging)")
	flag.Parse()

	loadStaticCommands()

	// TODO dggchat lib isn't flexible with the cookie name, workaround...
	dgg, err := dggchat.New(";jwt=" + authCookie)
	if err != nil {
		log.Fatalln(err)
	}

	// init bot
	b := newBot(authCookie, 250)
	b.addParser(
		b.staticMessage,
		b.nuke,
		b.aegis,
		b.noShortMsgSpam,
		b.rename,
		b.say,
		b.addCommand,
		b.mute,
		b.unmute,
		b.printTopStreams,
		b.modifyStream,
		b.checkAT,
		b.dropAT,
		b.provideAltAngelthumpLink,
		b.ban,
		b.sudoku,
		b.frenchToastAlert,
	)
	dgg.AddMessageHandler(b.onMessage)
	dgg.AddErrorHandler(b.onError)
	dgg.AddMuteHandler(b.onMute)
	dgg.AddUnmuteHandler(b.onUnmute)
	dgg.AddBanHandler(b.onBan)
	dgg.AddUnbanHandler(b.onUnban)
	dgg.AddSocketErrorHandler(b.onSocketError)
	dgg.AddPMHandler(b.onPMHandler)

	u, err := url.Parse(chatURL)
	if err != nil {
		log.Fatalln(err)
	}
	dgg.SetURL(*u)

	err = dgg.Open()
	if err != nil {
		log.Fatalln(err)
	}
	debuglogger.Println("[##] connected...")
	defer dgg.Close()

	info, err := b.getProfileInfo()
	if err != nil {
		debuglogger.Printf("userinfo: %s\n", err.Error())
	} else {
		debuglogger.Printf("userinfo: '%+v'\n", info)
	}

	// log to file and stdout
	logFile = reOpenLog()
	log.Println("[##] Restart")

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	if logOnly {
		debuglogger.Println("[##] started in logonly mode.")
	}
	debuglogger.Println("[##] waiting for signals...")
	for {
		sig := <-signals
		switch sig {

		// handle logrotate request from daemon
		case syscall.SIGHUP:
			log.Println("[##] signal: handling SIGHUP")
			err = logFile.Close()
			if err != nil {
				panic(err)
			}
			logFile = reOpenLog()

		// exit on interrupt
		case syscall.SIGTERM:
			fallthrough
		case syscall.SIGINT:
			log.Println("[##] signal: handling SIGINT/SIGTERM")
			err = logFile.Close()
			if err != nil {
				log.Printf("[##] error in cleanup: %s\n", err.Error())
			}
			os.Exit(1)
		}
	}
}

func reOpenLog() *os.File {
	dir, _ := path.Split(logFileName)
	if !fileExists(dir) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			panic(err)
		}
	}

	f, err := os.OpenFile(logFileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o755)
	if err != nil {
		panic(err)
	}
	mw := io.MultiWriter(os.Stdout, f)
	log.SetOutput(mw)
	return f
}

func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func loadStaticCommands() {
	if !fileExists(commandJSON) {
		log.Printf("creating empty commands file %s\n", commandJSON)
		os.Create(commandJSON)
		err := ioutil.WriteFile(commandJSON, []byte("{}"), 0o755)
		if err != nil {
			panic(err)
		}
	}

	b, err := ioutil.ReadFile(commandJSON)
	if err != nil {
		panic(err)
	}
	var cmnd map[string]string
	err = json.Unmarshal(b, &cmnd)
	if err != nil {
		panic(err)
	}
	commands = cmnd
}

func saveStaticCommands() bool {
	s, err := json.MarshalIndent(commands, "", "\t")
	if err != nil {
		log.Printf("failed marshaling commands, error: %v\n", err)
		return false
	}
	err = ioutil.WriteFile(commandJSON, s, 0o755)
	if err != nil {
		log.Printf("failed saving commands, error: %v\n", err)
		return false
	}
	return true
}
