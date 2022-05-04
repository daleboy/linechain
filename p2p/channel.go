package p2p

import (
	"context"
	"encoding/json"

	"github.com/libp2p/go-libp2p-core/peer"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const ChannelBufSize = 128
// Channel 的数据结构
type Channel struct {
	ctx   context.Context
	pub   *pubsub.PubSub//发布者
	topic *pubsub.Topic
	sub   *pubsub.Subscription//订阅者

	channelName string //构成Topic名称字符串的组成部分（TopicName="channel:" + channelName）
	self        peer.ID
	Content     chan *ChannelContent//ChannelContent类型的通道（带缓冲的通道，缓冲数量为 ChannelBufSize）
}

type ChannelContent struct {
	Message  string
	SendFrom string
	SendTo   string
	Payload  []byte
}

func JoinChannel(ctx context.Context, pub *pubsub.PubSub, selfID peer.ID, channelName string, subscribe bool) (*Channel, error) {
	// 加入发布者，以便于其它节点发现并连接
	// 所有类型的节点均可发布消息到订阅-发布系统中
	topic, err := pub.Join(topicName(channelName))
	if err != nil {
		return nil, err
	}

	var sub *pubsub.Subscription

	//确定订阅的节点，才会得到订阅-发布系统中的消息
	if subscribe {
		sub, err = topic.Subscribe()//订阅pubsub的pub消息
		if err != nil {
			return nil, err
		}
	} else {
		sub = nil
	}

	Channel := &Channel{
		ctx:         ctx,
		pub:         pub,
		topic:       topic,
		sub:         sub,
		self:        selfID,
		channelName: channelName,
		Content:     make(chan *ChannelContent, ChannelBufSize),//有缓冲的非阻塞通道，通道大小ChannelBufSize=128
	}

	//虽然所有的节点都会启动readLoop，但如果该节点没有订阅消息，是不会收到消息的
	go Channel.readLoop()//循环读取Channel中订阅的消息

	return Channel, nil
}

// ListPeers 列出该通信通道（主题Topic名称）中所有已经连接的节点
// 由发布者（pub）列举这些已连接的节点（节点启动时候，也会加入到pub中）
func (ch *Channel) ListPeers() []peer.ID {
	return ch.pub.ListPeers(topicName(ch.channelName))
}

// Publish 发布消息
func (channel *Channel) Publish(message string, payload []byte, SendTo string) error {
	m := ChannelContent{
		Message:  message,
		SendFrom: ShortID(channel.self),
		SendTo:   SendTo,
		Payload:  payload,
	}
	msgBytes, err := json.Marshal(m)
	if err != nil {
		return err
	}
	//从Publish实现来看，pub系统实际上也是直接将msgBytes发送到全网（包括本地节点），
	//并不先做解析，因此构建的消息结构只有Data和Topic赋值了，
	//From和Seqno均设置为nil。这意味着必然接受到消息
	return channel.topic.Publish(channel.ctx, msgBytes)
}

// readLoop 循环读取Channel中订阅的消息
func (channel *Channel) readLoop() {
	if channel.sub == nil {
		return
	}
	for {//无限循环
		content, err := channel.sub.Next(channel.ctx)
		if err != nil {
			close(channel.Content)
			return
		}
		// 仅转发其其它节点传递的消息（不包含自己发布的消息）
		if content.ReceivedFrom == channel.self {
			continue
		}

		NewContent := new(ChannelContent)
		//解析出消息的Data部分（结构与Publish函数创建的消息结构一致）
		err = json.Unmarshal(content.Data, NewContent)
		if err != nil {
			continue
		}

		//如果消息中的SendTo存在则为定向消息，该消息只能由SendTo节点处理，
		//如果SendTo与当前channel的peerId并不一致，说明该channel无需处理
		if NewContent.SendTo != "" && NewContent.SendTo != channel.self.Pretty() {
			continue
		}

		// 对于非定向消息（SendTo为空）或指定本channel接收到消息，则加入到该channel的消息队列中
		channel.Content <- NewContent
	}
}

func topicName(channelName string) string {
	return "channel:" + channelName
}
