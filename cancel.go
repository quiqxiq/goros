package routeros

import (
	"fmt"

	"github.com/go-routeros/routeros/v3/proto"
)

// cancelTag sends "/cancel =tag=<target>" to RouterOS directly on the wire.
// It registers a temporary sentenceProcessor so asyncLoop handles the !done reply.
func (c *Client) cancelTag(target string) {
	tag := fmt.Sprintf("c%d", c.incrementTag())

	a := &asyncReply{}
	a.tag = tag
	a.reC = make(chan *proto.Sentence, 1)

	c.mu.Lock()
	if c.tags == nil {
		c.mu.Unlock()
		return
	}
	c.tags[tag] = a
	c.mu.Unlock()

	c.w.BeginSentence()
	c.w.WriteWord("/cancel")
	c.w.WriteWord("=tag=" + target)
	c.w.WriteWord(".tag=" + tag)
	_ = c.w.EndSentence()
}
