/*
  Copyright JS Foundation and other contributors, https://js.foundation/

  Redistribution and use in source and binary forms, with or without
  modification, are permitted provided that the following conditions are met:

    * Redistributions of source code must retain the above copyright
      notice, this list of conditions and the following disclaimer.
    * Redistributions in binary form must reproduce the above copyright
      notice, this list of conditions and the following disclaimer in the
      documentation and/or other materials provided with the distribution.

  THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
  AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
  IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
  ARE DISCLAIMED. IN NO EVENT SHALL <COPYRIGHT HOLDER> BE LIABLE FOR ANY
  DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
  (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
  LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
  ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
  (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF
  THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

// Go port of src/comment-handler.ts.

package esprima

// commentHandler collects comments and, when attach is set, attaches them to
// the nodes as leadingComments, trailingComments and innerComments while the
// nodes are finished.
type commentHandler struct {
	attach   bool
	comments []any
	stack    []nodeInfo
	leading  []*commentEntry
	trailing []*commentEntry
}

type nodeInfo struct {
	node  *Node
	start int
}

type commentEntry struct {
	comment *Node
	start   int
}

func rangeStart(comment *Node) int {
	return comment.list("range")[0].(int)
}

func rangeEnd(comment *Node) int {
	return comment.list("range")[1].(int)
}

// insertInnerComments gives an empty block the comments inside it, as in
// `function a() {/** comments **/}`.
func (h *commentHandler) insertInnerComments(node *Node, meta Metadata) {
	if node.Type() != "BlockStatement" || len(node.list("body")) != 0 {
		return
	}
	var innerComments []any
	for i := len(h.leading) - 1; i >= 0; i-- {
		entry := h.leading[i]
		if meta.End.Offset >= entry.start {
			innerComments = append([]any{entry.comment}, innerComments...)
			h.leading = append(h.leading[:i], h.leading[i+1:]...)
			if i < len(h.trailing) {
				h.trailing = append(h.trailing[:i], h.trailing[i+1:]...)
			}
		}
	}
	if len(innerComments) > 0 {
		node.Set("innerComments", innerComments)
	}
}

func (h *commentHandler) findTrailingComments(meta Metadata) []any {
	trailingComments := []any{}

	if len(h.trailing) > 0 {
		for i := len(h.trailing) - 1; i >= 0; i-- {
			entry := h.trailing[i]
			if entry.start >= meta.End.Offset {
				trailingComments = append([]any{entry.comment}, trailingComments...)
			}
		}
		h.trailing = h.trailing[:0]
		return trailingComments
	}

	if len(h.stack) > 0 {
		last := h.stack[len(h.stack)-1]
		if existing := last.node.list("trailingComments"); last.node.Has("trailingComments") && existing != nil {
			if len(existing) > 0 && rangeStart(existing[0].(*Node)) >= meta.End.Offset {
				trailingComments = existing
				last.node.Delete("trailingComments")
			}
		}
	}
	return trailingComments
}

func (h *commentHandler) findLeadingComments(meta Metadata) []any {
	leadingComments := []any{}

	var target *Node
	for len(h.stack) > 0 {
		entry := h.stack[len(h.stack)-1]
		if entry.start >= meta.Start.Offset {
			target = entry.node
			h.stack = h.stack[:len(h.stack)-1]
		} else {
			break
		}
	}

	if target != nil {
		comments := target.list("leadingComments")
		for i := len(comments) - 1; i >= 0; i-- {
			comment := comments[i].(*Node)
			if rangeEnd(comment) <= meta.Start.Offset {
				leadingComments = append([]any{comment}, leadingComments...)
				comments = append(comments[:i], comments[i+1:]...)
			}
		}
		if target.Has("leadingComments") {
			if len(comments) == 0 {
				target.Delete("leadingComments")
			} else {
				target.Set("leadingComments", comments)
			}
		}
		return leadingComments
	}

	for i := len(h.leading) - 1; i >= 0; i-- {
		entry := h.leading[i]
		if entry.start <= meta.Start.Offset {
			leadingComments = append([]any{entry.comment}, leadingComments...)
			h.leading = append(h.leading[:i], h.leading[i+1:]...)
		}
	}
	return leadingComments
}

func (h *commentHandler) visitNode(node *Node, meta Metadata) {
	if node.Type() == "Program" && len(node.list("body")) > 0 {
		return
	}

	h.insertInnerComments(node, meta)
	trailingComments := h.findTrailingComments(meta)
	leadingComments := h.findLeadingComments(meta)
	if len(leadingComments) > 0 {
		node.Set("leadingComments", leadingComments)
	}
	if len(trailingComments) > 0 {
		node.Set("trailingComments", trailingComments)
	}

	h.stack = append(h.stack, nodeInfo{node: node, start: meta.Start.Offset})
}

func (h *commentHandler) visitComment(node *Node, meta Metadata) {
	typ := "Block"
	if node.Type()[0] == 'L' {
		typ = "Line"
	}
	comment := newNode("", "type", typ, "value", node.Get("value"))
	if node.Has("range") {
		comment.Set("range", node.Get("range"))
	}
	if node.Has("loc") {
		comment.Set("loc", node.Get("loc"))
	}
	h.comments = append(h.comments, comment)

	if h.attach {
		entry := &commentEntry{
			comment: newNode("", "type", typ, "value", node.Get("value"), "range", []any{meta.Start.Offset, meta.End.Offset}),
			start:   meta.Start.Offset,
		}
		if node.Has("loc") {
			entry.comment.Set("loc", node.Get("loc"))
		}
		node.Set("type", typ)
		h.leading = append(h.leading, entry)
		h.trailing = append(h.trailing, entry)
	}
}

func (h *commentHandler) visit(node *Node, meta Metadata) {
	switch node.Type() {
	case "LineComment", "BlockComment":
		h.visitComment(node, meta)
	default:
		if h.attach {
			h.visitNode(node, meta)
		}
	}
}
