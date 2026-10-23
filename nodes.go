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

// Go port of src/nodes.ts and src/jsx-nodes.ts.

package esprima

// Node constructors. Each sets the properties in the order the original's
// node classes assign them; the class is the original constructor name.

func nArrayExpression(elements []any) *Node {
	return newNode("ArrayExpression", "type", "ArrayExpression", "elements", elements)
}

func nArrayPattern(elements []any) *Node {
	return newNode("ArrayPattern", "type", "ArrayPattern", "elements", elements)
}

func nArrowFunctionExpression(params []any, body *Node, expression, async bool) *Node {
	class := "ArrowFunctionExpression"
	if async {
		class = "AsyncArrowFunctionExpression"
	}
	return newNode(class, "type", "ArrowFunctionExpression", "id", nil, "params", params, "body", body,
		"generator", false, "expression", expression, "async", async)
}

func nAssignmentExpression(operator string, left, right *Node) *Node {
	return newNode("AssignmentExpression", "type", "AssignmentExpression", "operator", operator, "left", left, "right", right)
}

func nAssignmentPattern(left, right *Node) *Node {
	return newNode("AssignmentPattern", "type", "AssignmentPattern", "left", left, "right", right)
}

func nAwaitExpression(argument *Node) *Node {
	return newNode("AwaitExpression", "type", "AwaitExpression", "argument", argument)
}

func nBinaryExpression(operator string, left, right any) *Node {
	typ := "BinaryExpression"
	if operator == "||" || operator == "&&" || operator == "??" {
		typ = "LogicalExpression"
	}
	return newNode("BinaryExpression", "type", typ, "operator", operator, "left", left, "right", right)
}

func nBlockStatement(body []any) *Node {
	return newNode("BlockStatement", "type", "BlockStatement", "body", body)
}

func nBreakStatement(label *Node) *Node {
	return newNode("BreakStatement", "type", "BreakStatement", "label", nilNode(label))
}

func nCallExpression(callee *Node, args []any, optional bool) *Node {
	return newNode("CallExpression", "type", "CallExpression", "callee", callee, "arguments", args, "optional", optional)
}

func nCatchClause(param, body *Node) *Node {
	return newNode("CatchClause", "type", "CatchClause", "param", nilNode(param), "body", body)
}

func nChainExpression(expression *Node) *Node {
	return newNode("ChainExpression", "type", "ChainExpression", "expression", expression)
}

func nClassBody(body []any) *Node {
	return newNode("ClassBody", "type", "ClassBody", "body", body)
}

func nClass(typ string, id, superClass, body *Node) *Node {
	return newNode(typ, "type", typ, "id", nilNode(id), "superClass", nilNode(superClass), "body", body)
}

func nComputedMemberExpression(object, property *Node, optional bool) *Node {
	return newNode("ComputedMemberExpression", "type", "MemberExpression", "computed", true, "object", object,
		"property", property, "optional", optional)
}

func nConditionalExpression(test, consequent, alternate *Node) *Node {
	return newNode("ConditionalExpression", "type", "ConditionalExpression", "test", test, "consequent", consequent,
		"alternate", alternate)
}

func nContinueStatement(label *Node) *Node {
	return newNode("ContinueStatement", "type", "ContinueStatement", "label", nilNode(label))
}

func nDebuggerStatement() *Node {
	return newNode("DebuggerStatement", "type", "DebuggerStatement")
}

func nDirective(expression *Node, directive string) *Node {
	return newNode("Directive", "type", "ExpressionStatement", "expression", expression, "directive", directive)
}

func nDoWhileStatement(body, test *Node) *Node {
	return newNode("DoWhileStatement", "type", "DoWhileStatement", "body", body, "test", test)
}

func nEmptyStatement() *Node {
	return newNode("EmptyStatement", "type", "EmptyStatement")
}

func nExportAllDeclaration(source *Node) *Node {
	return newNode("ExportAllDeclaration", "type", "ExportAllDeclaration", "source", source)
}

func nExportDefaultDeclaration(declaration *Node) *Node {
	return newNode("ExportDefaultDeclaration", "type", "ExportDefaultDeclaration", "declaration", declaration)
}

func nExportNamedDeclaration(declaration *Node, specifiers []any, source *Node) *Node {
	return newNode("ExportNamedDeclaration", "type", "ExportNamedDeclaration", "declaration", nilNode(declaration),
		"specifiers", specifiers, "source", nilNode(source))
}

func nExportSpecifier(local, exported *Node) *Node {
	return newNode("ExportSpecifier", "type", "ExportSpecifier", "exported", exported, "local", local)
}

func nExpressionStatement(expression *Node) *Node {
	return newNode("ExpressionStatement", "type", "ExpressionStatement", "expression", expression)
}

func nForInStatement(left, right, body *Node) *Node {
	return newNode("ForInStatement", "type", "ForInStatement", "left", left, "right", right, "body", body, "each", false)
}

func nForOfStatement(left, right, body *Node, await bool) *Node {
	return newNode("ForOfStatement", "type", "ForOfStatement", "await", await, "left", left, "right", right, "body", body)
}

func nForStatement(init, test, update, body *Node) *Node {
	return newNode("ForStatement", "type", "ForStatement", "init", nilNode(init), "test", nilNode(test),
		"update", nilNode(update), "body", body)
}

// nFunction builds FunctionDeclaration and FunctionExpression nodes, async or
// not.
func nFunction(typ string, id *Node, params []any, body *Node, generator, async bool) *Node {
	class := typ
	if async {
		class = "Async" + typ
	}
	return newNode(class, "type", typ, "id", nilNode(id), "params", params, "body", body,
		"generator", generator, "expression", false, "async", async)
}

func nIdentifier(name string) *Node {
	return newNode("Identifier", "type", "Identifier", "name", name)
}

func nIfStatement(test, consequent, alternate *Node) *Node {
	return newNode("IfStatement", "type", "IfStatement", "test", test, "consequent", consequent, "alternate", nilNode(alternate))
}

func nImport() *Node {
	return newNode("Import", "type", "Import")
}

func nImportDeclaration(specifiers []any, source *Node) *Node {
	return newNode("ImportDeclaration", "type", "ImportDeclaration", "specifiers", specifiers, "source", source)
}

func nImportDefaultSpecifier(local *Node) *Node {
	return newNode("ImportDefaultSpecifier", "type", "ImportDefaultSpecifier", "local", local)
}

func nImportNamespaceSpecifier(local *Node) *Node {
	return newNode("ImportNamespaceSpecifier", "type", "ImportNamespaceSpecifier", "local", local)
}

func nImportSpecifier(local, imported *Node) *Node {
	return newNode("ImportSpecifier", "type", "ImportSpecifier", "local", local, "imported", imported)
}

func nLabeledStatement(label, body *Node) *Node {
	return newNode("LabeledStatement", "type", "LabeledStatement", "label", label, "body", body)
}

func nLiteral(value any, raw string) *Node {
	return newNode("Literal", "type", "Literal", "value", value, "raw", raw)
}

func nMetaProperty(meta, property *Node) *Node {
	return newNode("MetaProperty", "type", "MetaProperty", "meta", meta, "property", property)
}

func nMethodDefinition(key *Node, computed bool, value *Node, kind string, isStatic bool) *Node {
	return newNode("MethodDefinition", "type", "MethodDefinition", "key", nilNode(key), "computed", computed,
		"value", nilNode(value), "kind", kind, "static", isStatic)
}

func nProgram(body []any, sourceType string) *Node {
	class := "Script"
	if sourceType == "module" {
		class = "Module"
	}
	return newNode(class, "type", "Program", "body", body, "sourceType", sourceType)
}

func nNewExpression(callee *Node, args []any) *Node {
	return newNode("NewExpression", "type", "NewExpression", "callee", callee, "arguments", args)
}

func nObjectExpression(properties []any) *Node {
	return newNode("ObjectExpression", "type", "ObjectExpression", "properties", properties)
}

func nObjectPattern(properties []any) *Node {
	return newNode("ObjectPattern", "type", "ObjectPattern", "properties", properties)
}

func nProperty(kind string, key *Node, computed bool, value *Node, method, shorthand bool) *Node {
	return newNode("Property", "type", "Property", "key", nilNode(key), "computed", computed, "value", nilNode(value),
		"kind", kind, "method", method, "shorthand", shorthand)
}

func nRegexLiteral(value *RegExp, raw, pattern, flags string) *Node {
	var v any
	if value != nil {
		v = value
	}
	return newNode("RegexLiteral", "type", "Literal", "value", v, "raw", raw,
		"regex", newNode("", "pattern", pattern, "flags", flags))
}

func nRestElement(argument *Node) *Node {
	return newNode("RestElement", "type", "RestElement", "argument", argument)
}

func nReturnStatement(argument *Node) *Node {
	return newNode("ReturnStatement", "type", "ReturnStatement", "argument", nilNode(argument))
}

func nSequenceExpression(expressions []any) *Node {
	return newNode("SequenceExpression", "type", "SequenceExpression", "expressions", expressions)
}

func nSpreadElement(argument *Node) *Node {
	return newNode("SpreadElement", "type", "SpreadElement", "argument", argument)
}

func nStaticMemberExpression(object, property *Node, optional bool) *Node {
	return newNode("StaticMemberExpression", "type", "MemberExpression", "computed", false, "object", object,
		"property", property, "optional", optional)
}

func nSuper() *Node {
	return newNode("Super", "type", "Super")
}

func nSwitchCase(test *Node, consequent []any) *Node {
	return newNode("SwitchCase", "type", "SwitchCase", "test", nilNode(test), "consequent", consequent)
}

func nSwitchStatement(discriminant *Node, cases []any) *Node {
	return newNode("SwitchStatement", "type", "SwitchStatement", "discriminant", discriminant, "cases", cases)
}

func nTaggedTemplateExpression(tag, quasi *Node) *Node {
	return newNode("TaggedTemplateExpression", "type", "TaggedTemplateExpression", "tag", tag, "quasi", quasi)
}

func nTemplateElement(raw string, cooked any, tail bool) *Node {
	return newNode("TemplateElement", "type", "TemplateElement", "value", newNode("", "raw", raw, "cooked", cooked), "tail", tail)
}

func nTemplateLiteral(quasis, expressions []any) *Node {
	return newNode("TemplateLiteral", "type", "TemplateLiteral", "quasis", quasis, "expressions", expressions)
}

func nThisExpression() *Node {
	return newNode("ThisExpression", "type", "ThisExpression")
}

func nThrowStatement(argument *Node) *Node {
	return newNode("ThrowStatement", "type", "ThrowStatement", "argument", argument)
}

func nTryStatement(block, handler, finalizer *Node) *Node {
	return newNode("TryStatement", "type", "TryStatement", "block", block, "handler", nilNode(handler), "finalizer", nilNode(finalizer))
}

func nUnaryExpression(operator string, argument *Node) *Node {
	return newNode("UnaryExpression", "type", "UnaryExpression", "operator", operator, "argument", argument, "prefix", true)
}

func nUpdateExpression(operator string, argument *Node, prefix bool) *Node {
	return newNode("UpdateExpression", "type", "UpdateExpression", "operator", operator, "argument", argument, "prefix", prefix)
}

func nVariableDeclaration(declarations []any, kind string) *Node {
	return newNode("VariableDeclaration", "type", "VariableDeclaration", "declarations", declarations, "kind", kind)
}

func nVariableDeclarator(id, init *Node) *Node {
	return newNode("VariableDeclarator", "type", "VariableDeclarator", "id", id, "init", nilNode(init))
}

func nWhileStatement(test, body *Node) *Node {
	return newNode("WhileStatement", "type", "WhileStatement", "test", test, "body", body)
}

func nWithStatement(object, body *Node) *Node {
	return newNode("WithStatement", "type", "WithStatement", "object", object, "body", body)
}

func nYieldExpression(argument *Node, delegate bool) *Node {
	return newNode("YieldExpression", "type", "YieldExpression", "argument", nilNode(argument), "delegate", delegate)
}

// JSX nodes.

func nJSXClosingElement(name *Node) *Node {
	return newNode("JSXClosingElement", "type", "JSXClosingElement", "name", name)
}

func nJSXClosingFragment() *Node {
	return newNode("JSXClosingFragment", "type", "JSXClosingFragment")
}

func nJSXElement(opening *Node, children []any, closing *Node) *Node {
	return newNode("JSXElement", "type", "JSXElement", "openingElement", opening, "children", children,
		"closingElement", nilNode(closing))
}

func nJSXEmptyExpression() *Node {
	return newNode("JSXEmptyExpression", "type", "JSXEmptyExpression")
}

func nJSXExpressionContainer(expression *Node) *Node {
	return newNode("JSXExpressionContainer", "type", "JSXExpressionContainer", "expression", expression)
}

func nJSXIdentifier(name string) *Node {
	return newNode("JSXIdentifier", "type", "JSXIdentifier", "name", name)
}

func nJSXMemberExpression(object, property *Node) *Node {
	return newNode("JSXMemberExpression", "type", "JSXMemberExpression", "object", object, "property", property)
}

func nJSXAttribute(name, value *Node) *Node {
	return newNode("JSXAttribute", "type", "JSXAttribute", "name", name, "value", nilNode(value))
}

func nJSXNamespacedName(namespace, name *Node) *Node {
	return newNode("JSXNamespacedName", "type", "JSXNamespacedName", "namespace", namespace, "name", name)
}

func nJSXOpeningElement(name *Node, selfClosing bool, attributes []any) *Node {
	return newNode("JSXOpeningElement", "type", "JSXOpeningElement", "name", name, "selfClosing", selfClosing,
		"attributes", attributes)
}

func nJSXOpeningFragment(selfClosing bool) *Node {
	return newNode("JSXOpeningFragment", "type", "JSXOpeningFragment", "selfClosing", selfClosing)
}

func nJSXSpreadAttribute(argument *Node) *Node {
	return newNode("JSXSpreadAttribute", "type", "JSXSpreadAttribute", "argument", argument)
}

func nJSXText(value, raw string) *Node {
	return newNode("JSXText", "type", "JSXText", "value", value, "raw", raw)
}

// nilNode turns a nil *Node into an untyped nil so it serialises as null.
func nilNode(n *Node) any {
	if n == nil {
		return nil
	}
	return n
}
