package mcts

import (
	"math"
	"sync"
)

type Node struct {
	ID           string
	Parent       *Node
	Children     []*Node
	Visits       int
	TotalReward  float64
	PatchDiff    string
	TargetFile   string
	Hypothesis   string
	RewardScore  float64
	ExecutionLog string
	Mu           sync.Mutex
}

func NewNode(id string, parent *Node, patchDiff, targetFile, hypothesis string) *Node {
	return &Node{
		ID:         id,
		Parent:     parent,
		Children:   make([]*Node, 0),
		PatchDiff:  patchDiff,
		TargetFile: targetFile,
		Hypothesis: hypothesis,
	}
}

// UCB1 computes Upper Confidence Bound to balance exploration and exploitation.
func (n *Node) UCB1(c float64) float64 {
	n.Mu.Lock()
	defer n.Mu.Unlock()

	if n.Visits == 0 {
		return math.MaxFloat64 // prioritize unvisited nodes
	}

	if n.Parent == nil {
		return n.TotalReward / float64(n.Visits)
	}

	exploitation := n.TotalReward / float64(n.Visits)
	exploration := c * math.Sqrt(math.Log(float64(n.Parent.Visits))/float64(n.Visits))
	return exploitation + exploration
}

// BestChild selects the child with highest UCB1 score.
func (n *Node) BestChild(c float64) *Node {
	n.Mu.Lock()
	defer n.Mu.Unlock()

	if len(n.Children) == 0 {
		return nil
	}

	var best *Node
	bestScore := -math.MaxFloat64

	for _, child := range n.Children {
		score := child.UCB1(c)
		if score > bestScore {
			bestScore = score
			best = child
		}
	}
	return best
}

// Backpropagate updates visit counts and rewards up to the root.
func (n *Node) Backpropagate(reward float64) {
	curr := n
	for curr != nil {
		curr.Mu.Lock()
		curr.Visits++
		curr.TotalReward += reward
		curr.Mu.Unlock()
		curr = curr.Parent
	}
}

// IsLeaf checks if node has no children.
func (n *Node) IsLeaf() bool {
	n.Mu.Lock()
	defer n.Mu.Unlock()
	return len(n.Children) == 0
}
