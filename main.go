package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	// SQLite3-Datenbanktreiber importieren
	_ "github.com/mattn/go-sqlite3"

	// Paket zum Generieren von QR-Codes importieren
	"github.com/mdp/qrterminal/v3"

	// WhatsApp-Paket importieren
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// MyClient ist eine Struktur, die eine WAClient-Instanz und eine Ereignis-Handler-ID enthält.
type MyClient struct {
	WAClient       *whatsmeow.Client
	eventHandlerID uint32
}

// Die Methode `register()` registriert den Ereignis-Handler.
func (mycli *MyClient) register() {
	// Ereignis-Handler mit der `AddEventHandler()`-Methode auf der WAClient-Instanz hinzufügen
	mycli.eventHandlerID = mycli.WAClient.AddEventHandler(mycli.eventHandler)
}

// Die Methode `eventHandler()` verarbeitet eingehende Nachrichten.
func (mycli *MyClient) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		// Ignore group messages
		// Group messages cause a panic, so we need to check if it's a group message
		if v.Info.IsGroup {
			return
		}
		// Support quoted messages
		// Whenever someone replies to your message by swiping left on it
		var newMessage *waProto.Message
		newMessage = v.Message
		quoted := newMessage.ExtendedTextMessage
		var msg string
		if quoted == nil {
			msg = newMessage.GetConversation()
		} else {
			msg = quoted.GetText()
		}
		fmt.Println("Message from:", v.Info.Sender.User, "->", msg)
		if msg == "" {
			return
		}
		// Make a http request to localhost:5001/chat?q= with the message, and send the response
		// URL encode the message
		urlEncoded := url.QueryEscape(msg)
		url := "http://127.0.0.1:8000/api/chat?q=" + urlEncoded
		// Make the request
		resp, err := http.Get(url)
		if err != nil {
			fmt.Println("Error making request:", err)
			return
		}
		// Read the response
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		newMsg := buf.String()
		// encode out as a string
		response := &waProto.Message{Conversation: proto.String(string(newMsg))}
		fmt.Println("Response:", response)

		userJid := types.NewJID(v.Info.Sender.User, types.DefaultUserServer)
		mycli.WAClient.SendMessage(context.Background(), userJid, response)

	}
}

func main() {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	// Make sure you add appropriate DB connector imports, e.g. github.com/mattn/go-sqlite3 for SQLite
	container, err := sqlstore.New("sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}
	// If you want multiple sessions, remember their JIDs and use .GetDevice(jid) or .GetAllDevices() instead.
	deviceStore, err := container.GetFirstDevice()

	if err != nil {
		panic(err)
	}

	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	// add the eventHandler
	mycli := &MyClient{WAClient: client}
	mycli.register()

	if client.Store.ID == nil {
		// No ID stored, new login
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()

		if err != nil {
			panic(err)
		}

		for evt := range qrChan {
			if evt.Event == "code" {
				// Render the QR code here
				// e.g. qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				// or just manually `echo 2@... | qrencode -t ansiutf8` in a terminal
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println("QR code:", evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		// Bereits eingeloggt, einfach verbinden
		err = client.Connect()
		if err != nil {
			panic(err)
		}
	}

	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	// Notify requires that there be enough capachttps://pkg.go.dev/os/signal#Notify
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	// Verbindung zum WhatsApp-Netzwerk trennen
	client.Disconnect()
}
