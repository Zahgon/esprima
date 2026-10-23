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

// Go port of src/messages.ts.

package esprima

// Error messages; they mirror V8's wording. %0 and %1 are placeholders.
const (
	msgAsyncFunctionInSingleStatementContext = "Async functions can only be declared at the top level or inside a block."
	msgBadImportCallArity                    = "Unexpected token"
	msgBadGetterArity                        = "Getter must not have any formal parameters"
	msgBadSetterArity                        = "Setter must have exactly one formal parameter"
	msgBadSetterRestParameter                = "Setter function argument must not be a rest parameter"
	msgCannotUseImportMetaOutsideAModule     = "Cannot use 'import.meta' outside a module"
	msgConstructorIsAsync                    = "Class constructor may not be an async method"
	msgConstructorSpecialMethod              = "Class constructor may not be an accessor"
	msgDeclarationMissingInitializer         = "Missing initializer in %0 declaration"
	msgDefaultRestParameter                  = "Unexpected token ="
	msgDefaultRestProperty                   = "Unexpected token ="
	msgDuplicateBinding                      = "Duplicate binding %0"
	msgDuplicateConstructor                  = "A class may only have one constructor"
	msgDuplicateParameter                    = "Duplicate parameter name not allowed in this context"
	msgDuplicateProtoProperty                = "Duplicate __proto__ fields are not allowed in object literals"
	msgForInOfLoopInitializer                = "%0 loop variable declaration may not have an initializer"
	msgGeneratorInLegacyContext              = "Generator declarations are not allowed in legacy contexts"
	msgIllegalBreak                          = "Illegal break statement"
	msgIllegalContinue                       = "Illegal continue statement"
	msgIllegalExportDeclaration              = "Unexpected token"
	msgIllegalImportDeclaration              = "Unexpected token"
	msgIllegalLanguageModeDirective          = "Illegal 'use strict' directive in function with non-simple parameter list"
	msgIllegalReturn                         = "Illegal return statement"
	msgInvalidEscapedReservedWord            = "Keyword must not contain escaped characters"
	msgInvalidHexEscapeSequence              = "Invalid hexadecimal escape sequence"
	msgInvalidLHSInAssignment                = "Invalid left-hand side in assignment"
	msgInvalidLHSInForIn                     = "Invalid left-hand side in for-in"
	msgInvalidLHSInForLoop                   = "Invalid left-hand side in for-loop"
	msgInvalidModuleSpecifier                = "Unexpected token"
	msgInvalidRegExp                         = "Invalid regular expression"
	msgInvalidTaggedTemplateOnOptionalChain  = "Invalid tagged template on optional chain"
	msgInvalidUnicodeEscapeSequence          = "Invalid Unicode escape sequence"
	msgLetInLexicalBinding                   = "let is disallowed as a lexically bound name"
	msgMissingFromClause                     = "Unexpected token"
	msgMultipleDefaultsInSwitch              = "More than one default clause in switch statement"
	msgNewlineAfterThrow                     = "Illegal newline after throw"
	msgNoAsAfterImportNamespace              = "Unexpected token"
	msgNoCatchOrFinally                      = "Missing catch or finally after try"
	msgParameterAfterRestParameter           = "Rest parameter must be last formal parameter"
	msgPropertyAfterRestProperty             = "Unexpected token"
	msgRedeclaration                         = "%0 '%1' has already been declared"
	msgStaticPrototype                       = "Classes may not have static property named prototype"
	msgStrictCatchVariable                   = "Catch variable may not be eval or arguments in strict mode"
	msgStrictDelete                          = "Delete of an unqualified identifier in strict mode."
	msgStrictFunction                        = "In strict mode code, functions can only be declared at top level or inside a block"
	msgStrictFunctionName                    = "Function name may not be eval or arguments in strict mode"
	msgStrictLHSAssignment                   = "Assignment to eval or arguments is not allowed in strict mode"
	msgStrictLHSPostfix                      = "Postfix increment/decrement may not have eval or arguments operand in strict mode"
	msgStrictLHSPrefix                       = "Prefix increment/decrement may not have eval or arguments operand in strict mode"
	msgStrictModeWith                        = "Strict mode code may not include a with statement"
	msgStrictOctalLiteral                    = "Octal literals are not allowed in strict mode."
	msgStrictParamName                       = "Parameter name eval or arguments is not allowed in strict mode"
	msgStrictReservedWord                    = "Use of future reserved word in strict mode"
	msgStrictVarName                         = "Variable name may not be eval or arguments in strict mode"
	msgTemplateOctalLiteral                  = "Octal literals are not allowed in template strings."
	msgTemplateEscape89                      = "\\8 and \\9 are not allowed in template strings."
	msgUnexpectedEOS                         = "Unexpected end of input"
	msgUnexpectedIdentifier                  = "Unexpected identifier"
	msgUnexpectedNumber                      = "Unexpected number"
	msgUnexpectedReserved                    = "Unexpected reserved word"
	msgUnexpectedString                      = "Unexpected string"
	msgUnexpectedSuper                       = "'super' keyword unexpected here"
	msgUnexpectedTemplate                    = "Unexpected quasi %0"
	msgUnexpectedToken                       = "Unexpected token %0"
	msgUnexpectedTokenIllegal                = "Unexpected token ILLEGAL"
	msgUnknownLabel                          = "Undefined label '%0'"
	msgUnterminatedRegExp                    = "Invalid regular expression: missing /"

	msgJSXEmptyAttributeExpression = "JSX attributes must only be assigned a non-empty expression"
	msgJSXClosingTagMismatch       = "Expected corresponding JSX closing tag for %0"
	msgJSXFragmentClosingMismatch  = "Expected corresponding JSX closing tag for jsx fragment"
)
