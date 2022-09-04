package p2p

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rivo/tview"
	log "github.com/sirupsen/logrus"

	"github.com/gdamore/tcell/v2"
	"github.com/libp2p/go-libp2p/core/peer"
)

// CLIUI 是给对等端使用的文本用户界面（Text User Interface (TUI)）
// 应用了著名的开源项目tcell和tview
type CLIUI struct {
	GeneralChannel   *Channel
	MiningChannel    *Channel
	FullNodesChannel *Channel
	app              *tview.Application
	peersList        *tview.TextView

	hostWindow *tview.TextView
	inputCh    chan string   //带缓冲的通道，缓冲数量1
	doneCh     chan struct{} //带缓冲的通道，缓冲数量32
}

// log的默认结构包含：level、msg、time三个属性
type Log struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
	Time  string `json:"time"`
}

var (
	_, b, _, _ = runtime.Caller(0)

	// 项目根目录
	Root = filepath.Join(filepath.Dir(b), "../")
)

func NewCLIUI(generalChannel *Channel, miningChannel *Channel, fullNodesChannel *Channel) *CLIUI {
	app := tview.NewApplication()

	msgBox := tview.NewTextView()
	msgBox.SetDynamicColors(true)
	msgBox.SetBorder(true)
	msgBox.SetTitle(fmt.Sprintf("HOST (%s)", strings.ToUpper(ShortID(generalChannel.self))))

	msgBox.SetChangedFunc(func() {
		app.Draw()
	})

	inputCh := make(chan string, 32)
	input := tview.NewInputField().
		SetLabel(strings.ToUpper(ShortID(generalChannel.self)) + " > ").
		SetFieldWidth(0).
		SetFieldBackgroundColor(tcell.ColorBlack)

	input.SetDoneFunc(func(key tcell.Key) {
		if key != tcell.KeyEnter {
			// 非Enter键输入，不做任何事
			return
		}
		line := input.GetText()
		if len(line) == 0 {
			// 忽略空白行
			return
		}

		// 释放
		if line == "/quit" {
			app.Stop()
			return
		}

		inputCh <- line
		input.SetText("")
	})

	peersList := tview.NewTextView()
	peersList.SetBorder(true)
	peersList.SetTitle("Peers")

	chatPanel := tview.NewFlex().
		AddItem(msgBox, 0, 1, false).
		AddItem(peersList, 20, 1, false)

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(chatPanel, 0, 1, false)

	app.SetRoot(flex, true)

	return &CLIUI{
		GeneralChannel:   generalChannel,
		MiningChannel:    miningChannel,
		FullNodesChannel: fullNodesChannel,
		app:              app,
		peersList:        peersList,
		hostWindow:       msgBox,
		inputCh:          inputCh,
		doneCh:           make(chan struct{}, 1),
	}
}

// Run 在后台开启日志事件循环，然后为文本UI开启事件循环
func (ui *CLIUI) Run(net *Network) error {
	go ui.handleEvents(net)
	defer ui.end()

	return ui.app.Run()
}

// End表示事件循环正常退出
func (ui *CLIUI) end() {
	ui.doneCh <- struct{}{}
}

// refreshPeers 拉取当前在channel中的peer list，并显示peer id的最后八个字符在UI上
func (ui *CLIUI) refreshPeers() {
	peers := ui.GeneralChannel.ListPeers()
	minerPeers := ui.MiningChannel.ListPeers()
	idStrs := make([]string, len(peers))

	for i, p := range peers {
		peerId := strings.ToUpper(ShortID(p))
		if len(minerPeers) != 0 {
			isMiner := false
			for _, minerPeer := range minerPeers {
				if minerPeer == p {
					isMiner = true
					break
				}
			}
			if isMiner {
				idStrs[i] = "MINER: " + peerId
			} else {
				idStrs[i] = peerId
			}
		} else {
			idStrs[i] = peerId
		}
	}

	ui.peersList.SetText(strings.Join(idStrs, "\n"))
	ui.app.Draw()
}

func (ui *CLIUI) displaySelfMessage(msg string) {
	prompt := withColor("yellow", fmt.Sprintf("<%s>:", strings.ToUpper(ShortID(ui.GeneralChannel.self))))
	fmt.Fprintf(ui.hostWindow, "%s %s\n", prompt, msg)
}

// func (ui *CLIUI) displayContent(content *ChannelContent) {
// 	prompt := withColor("green", fmt.Sprintf("<%s>:", strings.ToUpper(content.SendFrom)))
// 	fmt.Fprintf(ui.hostWindow, "%s %s\n", prompt, content.Message)
// }

// HandleStream 真正处理来自全网发来的且可以处理的消息内容
func (ui *CLIUI) HandleStream(net *Network, content *ChannelContent) {
	// ui.displayContent(content)
	if content.Payload != nil {
		command := BytesToCmd(content.Payload[:commandLength])
		log.Infof("Received  %s command \n", command)

		switch command {
		case "block":
			net.HandleBlock(content)
		case "inv":
			net.HandleInv(content)
		case "getblocks":
			net.HandleGetBlocks(content)
		case "getdata":
			net.HandleGetData(content)
		case "tx":
			net.HandleTx(content)
		case "gettxfrompool":
			net.HandleGetTxFromPool(content)
		case "version":
			net.HandleVersion(content)
		default:
			log.Warn("Unknown Command")
		}
	}
}

func (ui *CLIUI) readFromLogs(instanceId string) {
	filename := "/logs/console.log"
	if instanceId != "" {
		filename = fmt.Sprintf("/logs/console_%s.log", instanceId)
	}

	logFile := path.Join(Root, filename)
	e := ioutil.WriteFile(logFile, []byte(""), 0644)
	if e != nil {
		panic(e)
	}
	log.SetOutput(ioutil.Discard)

	f, err := os.Open(logFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	r := bufio.NewReader(f)
	info, err := f.Stat()
	if err != nil {
		panic(err)
	}
	logLevels := map[string]string{
		"info":    "green",
		"warning": "brown",
		"error":   "red",
		"fatal":   "red",
	}
	oldSize := info.Size()
	for {
		for line, prefix, err := r.ReadLine(); err != io.EOF; line, prefix, err = r.ReadLine() {
			var data Log
			if err := json.Unmarshal(line, &data); err != nil {
				panic(err)
			}
			prompt := fmt.Sprintf("[%s]:", withColor(logLevels[data.Level], strings.ToUpper(data.Level)))
			if prefix {
				fmt.Fprintf(ui.hostWindow, "%s %s\n", prompt, data.Msg)
			} else {
				fmt.Fprintf(ui.hostWindow, "%s %s\n", prompt, data.Msg)
			}
			ui.hostWindow.ScrollToEnd()
		}
		pos, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			panic(err)
		}
		for {
			time.Sleep(time.Second)
			newinfo, err := f.Stat()
			if err != nil {
				panic(err)
			}
			newSize := newinfo.Size()
			if newSize != oldSize {
				if newSize < oldSize {
					f.Seek(0, 0)
				} else {
					f.Seek(pos, io.SeekStart)
				}
				r = bufio.NewReader(f)
				oldSize = newSize
				break
			}
		}
	}
}

// handleEvents 运行一个事件循环，以将用户输入发送到channel中,并且显示从channel中接收到的消息。
// 它也定期地在UI刷新peers list
// 同时接收三个通道的消息并进行处理（注意三个通道消息接收后，处理函数相同，均为HandleStream）
func (ui *CLIUI) handleEvents(net *Network) {
	peerRefreshTicker := time.NewTicker(time.Second)
	defer peerRefreshTicker.Stop()

	go ui.readFromLogs(net.Blockchain.InstanceId)
	log.Info("HOST ADDR: ", net.Host.Addrs())

	for {
		select {
		case input := <-ui.inputCh:
			err := ui.GeneralChannel.Publish(input, nil, "") //未指定消息接收者，意味着所有节点均会收到
			if err != nil {
				log.Errorf("Publish error: %s", err)
			}
			ui.displaySelfMessage(input)

		case <-peerRefreshTicker.C:
			// 定期刷新peers list
			ui.refreshPeers()

		case m := <-ui.GeneralChannel.Content: //如果 GeneralChannel 收到消息
			ui.HandleStream(net, m)

		case m := <-ui.MiningChannel.Content: //如果 MiningChannel 收到消息
			ui.HandleStream(net, m)

		case m := <-ui.FullNodesChannel.Content: //如果 FullNodesChannel 收到消息
			ui.HandleStream(net, m)

		case <-ui.GeneralChannel.ctx.Done():
			return

		case <-ui.doneCh:
			return
		}
	}
}

// withColor 使用color tags封装字符串以显示在UI的消息文本框中
func withColor(color, msg string) string {
	return fmt.Sprintf("[%s]%s[-]", color, msg)
}

// ShortID 返回一个base58-encoded peer id的最后八个字符
func ShortID(p peer.ID) string {
	pretty := p.Pretty()
	return pretty[len(pretty)-8:]
}
