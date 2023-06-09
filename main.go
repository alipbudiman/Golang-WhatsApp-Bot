package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal"
	"github.com/sirupsen/logrus"

	"./metadevlibs/botlib"
	"./metadevlibs/feature"
	"./metadevlibs/helper"
	"./metadevlibs/object"
	"./metadevlibs/transport"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	waLog "go.mau.fi/whatsmeow/util/log"
)

type ClientWrapper struct {
	*botlib.CLIENT
}

var (
	Log           *logrus.Logger
	Client        *ClientWrapper
	myJID         types.JID
	ChatGPTApikey string = "" // << INSERT YOUR OPEN AI API HERE
	ChatGPTProxy  string = ""
)

func (cl *ClientWrapper) MessageHandler(evt interface{}) {
	helper.TrackEvents(evt)
	switch v := evt.(type) {
	case *events.Message:
		text_in_ExtendedText := v.Message.ExtendedTextMessage.GetText()
		mobile_txt := v.Message.GetConversation()
		pc_txt := v.Message.ExtendedTextMessage.GetText()
		var txtV2 string
		var txt string
		var from_dm bool = false
		switch {
		case text_in_ExtendedText != "":
			txt = strings.ToLower(text_in_ExtendedText)
			txtV2 = text_in_ExtendedText
		case pc_txt != "":
			txt = strings.ToLower(pc_txt)
			txtV2 = pc_txt
		default:
			txt = strings.ToLower(mobile_txt)
			txtV2 = mobile_txt
		}
		sender := v.Info.Sender
		senderSTR := fmt.Sprintf("%v", sender)
		sender_jid, is_success := helper.SenderJIDConvert(sender)
		if is_success {
			sender = sender_jid
		}
		to := v.Info.Chat
		if to == sender {
			from_dm = true
		}
		if txt == "ping" {
			cl.SendTextMessage(to, "Pong!")
		} else if txt == "help" {
			cl.SendTextMessage(to, helper.WriteDisplayMenu(from_dm))
		} else if txt == "send image" {
			cl.SendTextMessage(to, "Loading . . .")
			cl.SendImageMessage(to, "assets/img/img.jpg", ">_<")
		} else if txt == "send video" {
			cl.SendTextMessage(to, "Loading . . .")
			cl.SendVideoMessage(to, "assets/vid/vid.mp4", ">_<")
		} else if strings.HasPrefix(txt, "say: ") {
			spl := txtV2[len("say: "):]
			cl.SendTextMessage(to, spl)
		} else if strings.HasPrefix(txt, "chat gpt: ") {
			cl.SendTextMessage(to, "Process . . .")
			question := txtV2[len("chat gpt: "):]
			responGPT, err := feature.ChatGPT(sender, question)
			if err != nil {
				cl.SendTextMessage(to, "Error please check console for detail")
				panic(err)
			}
			buildRespon := "*Chat GPT Response:*"
			buildRespon += "\n" + responGPT
			buildRespon += "\n\n____ [done] ____"
			for _, msg := range helper.LooperMessage(buildRespon, 2000) {
				cl.SendTextMessage(to, msg)
				time.Sleep(1 * time.Second)
			}
			buildConvertation := "*Convertation*"
			buildConvertation += "\nConvertation by: " + helper.MentionFormat(senderSTR)
			buildConvertation += fmt.Sprintf("\nTotal convertation: %d", (len(feature.GPTMap[sender])-1)/2)
			cl.SendMention(to, buildConvertation, []string{senderSTR})

		} else if strings.HasPrefix(txt, "dalle draw: ") {
			cl.SendTextMessage(to, "Process . . .")
			prompt := txtV2[len("dalle draw: "):]
			imgLoad := 3           // Total of DAll-E load image (max 10)
			imgSize := "1024x1024" // Generated images can have a size of "256x256", "512x512", or "1024x1024 "pixels
			responDallE, err := feature.DallE(prompt, imgLoad, imgSize)
			if err != nil {
				cl.SendTextMessage(to, "Error please check console for detail")
				panic(err)
			}
			for i, img := range responDallE {
				fileName := object.GenerateFileName(".jpg")
				data, err := transport.Download(img, "tmp/img", fileName)
				if err != nil {
					cl.SendTextMessage(to, fmt.Sprintf("Error Fail load image %d, please check console for detail", i+1))
					fmt.Sprintf(err.Error())
					continue
				}
				msg := fmt.Sprintf("Total Generate image: %d / %d", imgLoad, i+1)
				msg += fmt.Sprintf("\nTotal word in promp: %d", len(strings.Fields(prompt)))
				cl.SendImageMessage(to, data, msg)
				os.Remove(data)
			}
		}

		if !from_dm {
			if txt == "tag all" {
				cl.SendTextMessage(to, "Loading . . .")
				mem := cl.GetMemberList(to)
				mem = helper.RemoveMyJID(mem, myJID)
				ret := "⌬ Mentionall\n"
				for _, jid := range mem {
					ret += "\n- " + helper.MentionFormat(jid)
				}
				ret += fmt.Sprintf("\n\nTotal %v user", len(mem))
				cl.SendMention(to, ret, mem)

			} else if strings.HasPrefix(txt, "say: ") {
				msg := txtV2[len("say: "):]
				cl.SendTextMessage(to, msg)
			}
		}
		return
	default:
		fmt.Println(reflect.TypeOf(v))
		fmt.Println(v)
	}
}

func (cl *ClientWrapper) register() {
	cl.Client.AddEventHandler(cl.MessageHandler)
}

func (cl *ClientWrapper) newClient(d *store.Device, l waLog.Logger) {
	cl.Client = whatsmeow.NewClient(d, l)
}

func main() {
	feature.GPTConfig("", ChatGPTApikey, ChatGPTProxy)
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container, err := sqlstore.New("sqlite3", "file:commander.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	deviceStore, err := container.GetFirstDevice()
	makeJID, _ := helper.ConvertJID(fmt.Sprintf("%v", deviceStore.ID))
	Resjid, _ := helper.SenderJIDConvert(makeJID)
	myJID = Resjid

	if err != nil {
		panic(err)
	}

	clientLog := waLog.Stdout("Client", "DEBUG", true)
	Client = &ClientWrapper{
		CLIENT: &botlib.CLIENT{},
	}
	Client.newClient(deviceStore, clientLog)
	Client.register()

	if Client.Client.Store.ID == nil {
		qrChan, _ := Client.Client.GetQRChannel(context.Background())
		err = Client.Client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}

	} else {
		err = Client.Client.Connect()
		fmt.Println("Login Success")
		if err != nil {
			panic(err)
		}
	}
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	Client.Client.Disconnect()
}
