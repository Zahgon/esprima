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

// Go port of src/token.ts, src/syntax.ts and src/jsx-syntax.ts.

package esprima

// tokenType is the kind of a scanned token.
type tokenType int

const (
	tokBooleanLiteral tokenType = iota + 1
	tokEOF
	tokIdentifier
	tokKeyword
	tokNullLiteral
	tokNumericLiteral
	tokPunctuator
	tokStringLiteral
	tokRegularExpression
	tokTemplate

	// JSX-only token kinds.
	tokJSXIdentifier tokenType = 100
	tokJSXText       tokenType = 101
)

// tokenName is the name a token kind has in token lists.
func tokenName(t tokenType) string {
	switch t {
	case tokBooleanLiteral:
		return "Boolean"
	case tokEOF:
		return "<end>"
	case tokIdentifier:
		return "Identifier"
	case tokKeyword:
		return "Keyword"
	case tokNullLiteral:
		return "Null"
	case tokNumericLiteral:
		return "Numeric"
	case tokPunctuator:
		return "Punctuator"
	case tokStringLiteral:
		return "String"
	case tokRegularExpression:
		return "RegularExpression"
	case tokTemplate:
		return "Template"
	case tokJSXIdentifier:
		return "JSXIdentifier"
	case tokJSXText:
		return "JSXText"
	}
	return ""
}

// Syntax lists the node types of the syntax tree.
var Syntax = []string{
	"AssignmentExpression", "AssignmentPattern", "ArrayExpression", "ArrayPattern",
	"ArrowFunctionExpression", "AwaitExpression", "BlockStatement", "BinaryExpression",
	"BreakStatement", "CallExpression", "CatchClause", "ChainExpression", "ClassBody",
	"ClassDeclaration", "ClassExpression", "ConditionalExpression", "ContinueStatement",
	"DoWhileStatement", "DebuggerStatement", "EmptyStatement", "ExportAllDeclaration",
	"ExportDefaultDeclaration", "ExportNamedDeclaration", "ExportSpecifier", "ExpressionStatement",
	"ForStatement", "ForOfStatement", "ForInStatement", "FunctionDeclaration", "FunctionExpression",
	"Identifier", "IfStatement", "Import", "ImportDeclaration", "ImportDefaultSpecifier",
	"ImportNamespaceSpecifier", "ImportSpecifier", "Literal", "LabeledStatement", "LogicalExpression",
	"MemberExpression", "MetaProperty", "MethodDefinition", "NewExpression", "ObjectExpression",
	"ObjectPattern", "Program", "Property", "RestElement", "ReturnStatement", "SequenceExpression",
	"SpreadElement", "Super", "SwitchCase", "SwitchStatement", "TaggedTemplateExpression",
	"TemplateElement", "TemplateLiteral", "ThisExpression", "ThrowStatement", "TryStatement",
	"UnaryExpression", "UpdateExpression", "VariableDeclaration", "VariableDeclarator",
	"WhileStatement", "WithStatement", "YieldExpression",
}

// JSXSyntax lists the node types of JSX.
var JSXSyntax = []string{
	"JSXAttribute", "JSXClosingElement", "JSXClosingFragment", "JSXElement", "JSXEmptyExpression",
	"JSXExpressionContainer", "JSXIdentifier", "JSXMemberExpression", "JSXNamespacedName",
	"JSXOpeningElement", "JSXOpeningFragment", "JSXSpreadAttribute", "JSXText",
}

// Version is the version of the original Esprima this package reproduces.
const Version = "4.0.0-dev"
