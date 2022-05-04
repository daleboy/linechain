package blockchain

import (
	"crypto/sha256"

	log "github.com/sirupsen/logrus"
)

type MerkleTree struct {
	RootNode *MerkleNode
}

type MerkleNode struct {
	Right *MerkleNode
	Left  *MerkleNode
	Data  []byte
}

func NewMerkleNode(left, right *MerkleNode, data []byte) *MerkleNode {
	node := MerkleNode{}

	if left == nil && right == nil {
		hash := sha256.Sum256(data)
		node.Data = hash[:]
	} else {
		prevHashes := append(left.Data, right.Data...)
		hash := sha256.Sum256(prevHashes)
		node.Data = hash[:]
	}

	node.Left = left
	node.Right = right
	return &node
}

// 二进制类树实现
func NewMerkleTree(data [][]byte) *MerkleTree {

	var nodes []MerkleNode

	for _, d := range data {
		node := NewMerkleNode(nil, nil, d)
		nodes = append(nodes, *node)
	}

	if len(nodes) == 0 {
		log.Panic("No merkle Tree node")
	}

	for len(nodes) > 1 {
		// 叶节点的长度必须是偶数
		if len(nodes)%2 != 0 {
			// 将最后一个节点复制一份并加入到叶子节点列表中
			dupNode := nodes[len(nodes)-1]
			nodes = append(nodes, dupNode)
		}

		var level []MerkleNode
		for i := 0; i < len(nodes); i += 2 {
			node := NewMerkleNode(&nodes[i], &nodes[i+1], nil)
			level = append(level, *node)
		}

		nodes = level
	}

	tree := MerkleTree{&nodes[0]}

	return &tree
}
