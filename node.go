package esprima

// Node is a syntax tree node, token or comment: a JavaScript-style object
// whose properties keep their insertion order. The order is part of the
// output format (it is the order of the original's object properties), and
// the parser rewrites nodes in place the way the original does: patterns are
// reinterpreted by changing "type", and comment attachment adds and removes
// properties after a node has been finished.
//
// Property values are nil (null), bool, float64, string (a JavaScript string,
// see Units), *Node, []any, *RegExp or, for the errors of tolerant parsing,
// *Error.
type Node struct {
	keys []string
	vals []any
	// class names the constructor that created the node, standing in for the
	// original's instanceof checks (a node whose type was rewritten keeps its
	// class).
	class string
}

// newNode creates a node of the given constructor class from alternating
// property names and values.
func newNode(class string, kv ...any) *Node {
	n := &Node{class: class, keys: make([]string, 0, len(kv)/2+2), vals: make([]any, 0, len(kv)/2+2)}
	for i := 0; i < len(kv); i += 2 {
		n.keys = append(n.keys, kv[i].(string))
		n.vals = append(n.vals, kv[i+1])
	}
	return n
}

// ObjectOf creates a plain object from alternating property names and values,
// for building JSON output alongside syntax trees.
func ObjectOf(kv ...any) *Node {
	return newNode("", kv...)
}

// Type returns the node's "type" property.
func (n *Node) Type() string {
	if n == nil {
		return ""
	}
	t, _ := n.Get("type").(string)
	return t
}

// Keys returns the property names in order.
func (n *Node) Keys() []string {
	return append([]string(nil), n.keys...)
}

// Get returns a property value, or nil when the property is absent.
func (n *Node) Get(key string) any {
	for i, k := range n.keys {
		if k == key {
			return n.vals[i]
		}
	}
	return nil
}

// Has reports whether the property exists.
func (n *Node) Has(key string) bool {
	for _, k := range n.keys {
		if k == key {
			return true
		}
	}
	return false
}

// Set assigns a property: an existing property keeps its position, a new one
// is appended.
func (n *Node) Set(key string, v any) {
	for i, k := range n.keys {
		if k == key {
			n.vals[i] = v
			return
		}
	}
	n.keys = append(n.keys, key)
	n.vals = append(n.vals, v)
}

// Delete removes a property.
func (n *Node) Delete(key string) {
	for i, k := range n.keys {
		if k == key {
			n.keys = append(n.keys[:i], n.keys[i+1:]...)
			n.vals = append(n.vals[:i], n.vals[i+1:]...)
			return
		}
	}
}

// node returns a property holding a node, or nil.
func (n *Node) node(key string) *Node {
	c, _ := n.Get(key).(*Node)
	return c
}

// list returns a property holding an array, or nil.
func (n *Node) list(key string) []any {
	l, _ := n.Get(key).([]any)
	return l
}

// str returns a property holding a string, or "".
func (n *Node) str(key string) string {
	s, _ := n.Get(key).(string)
	return s
}

// RegExp is the value of a regular expression literal: the pattern and flags
// for which the host engine could construct a regular expression object.
type RegExp struct {
	Pattern string
	Flags   string
}

// String returns the RegExp's string form, as RegExp.prototype.toString
// gives it: the source with unescaped slashes and line terminators escaped,
// followed by the flags in canonical order.
func (r *RegExp) String() string {
	return "/" + escapeRegExpSource(r.Pattern) + "/" + canonicalFlags(r.Flags)
}

// Position is a line and column (zero-based column) in the source.
type Position struct {
	Line   int
	Column int
	Offset int
}

// Metadata is passed to a Delegate with the start and end of a node.
type Metadata struct {
	Start Position
	End   Position
}

// Delegate is called with every node as soon as it is finished, and with
// every comment when comments are collected.
type Delegate func(node *Node, meta Metadata)
