package call

import (
	"github.com/antlr/antlr4/runtime/Go/antlr"
	"github.com/phodal/coca/core/languages/java"
	"github.com/phodal/coca/core/models"
	"reflect"
	"strings"
)

var imports []string
var clzs []string
var currentPkg string
var currentClz string
var fields []models.JAppField
var methodCalls []models.JMethodCall
var currentType string

var mapFields = make(map[string]string)
var localVars = make(map[string]string)
var formalParameters = make(map[string]string)
var currentClzExtend = ""
var currentMethod models.JMethod
var methodMap = make(map[string]models.JMethod)

var methodQueue []models.JMethod
var classQueue []string

var identMap map[string]models.JIdentifier
var currentNode models.JClassNode

func NewJavaCallListener(nodes map[string]models.JIdentifier) *JavaCallListener {
	currentClz = ""
	currentPkg = ""
	currentMethod = models.NewJMethod()
	currentNode = models.NewClassNode()

	identMap = nodes

	methodMap = make(map[string]models.JMethod)

	methodCalls = nil
	fields = nil
	return &JavaCallListener{}
}

type JavaCallListener struct {
	parser.BaseJavaParserListener
}

func (s *JavaCallListener) getNodeInfo() models.JClassNode {
	var methodsArray []models.JMethod
	for _, value := range methodMap {
		methodsArray = append(methodsArray, value)
	}

	currentNode.MethodCalls = methodCalls
	currentNode.Fields = fields
	currentNode.Type = currentType
	currentNode.Methods = methodsArray
	return currentNode
}

func (s *JavaCallListener) EnterPackageDeclaration(ctx *parser.PackageDeclarationContext) {
	currentNode.Package = ctx.QualifiedName().GetText()
	currentPkg = ctx.QualifiedName().GetText()
}

func (s *JavaCallListener) EnterImportDeclaration(ctx *parser.ImportDeclarationContext) {
	importText := ctx.QualifiedName().GetText()
	imports = append(imports, importText)
}

func (s *JavaCallListener) EnterClassDeclaration(ctx *parser.ClassDeclarationContext) {
	currentType = "Class"
	if ctx.IDENTIFIER() != nil {
		currentClz = ctx.IDENTIFIER().GetText()
		currentNode.Class = currentClz
	}

	if ctx.EXTENDS() != nil {
		currentClzExtend = ctx.TypeType().GetText()
		for _, imp := range imports {
			if strings.HasSuffix(imp, "."+currentClzExtend) {
				currentNode.Extend = currentClzExtend
			}
		}
	}

	if ctx.IMPLEMENTS() != nil {
		types := ctx.TypeList().(*parser.TypeListContext).AllTypeType()
		for _, typ := range types {
			typeText := typ.GetText()
			var hasSetImplement = false
			for _, imp := range imports {
				if strings.HasSuffix(imp, "."+typeText) {
					hasSetImplement = true
					currentNode.Implements = append(currentNode.Implements, imp)
				}
			}
			// 同一个包下的类
			if !hasSetImplement {
				currentNode.Implements = append(currentNode.Implements, currentPkg+"."+typeText)
			}
		}
	}

	// TODO: 支持依赖注入
}

func (s *JavaCallListener) EnterInterfaceDeclaration(ctx *parser.InterfaceDeclarationContext) {
	currentType = "Interface"
	currentNode.Class = ctx.IDENTIFIER().GetText()
}

func (s *JavaCallListener) EnterInterfaceMethodDeclaration(ctx *parser.InterfaceMethodDeclarationContext) {
	startLine := ctx.GetStart().GetLine()
	startLinePosition := ctx.IDENTIFIER().GetSymbol().GetColumn()
	stopLine := ctx.GetStop().GetLine()
	name := ctx.IDENTIFIER().GetText()
	stopLinePosition := startLinePosition + len(name)

	typeType := ctx.TypeTypeOrVoid().GetText()

	method := &models.JMethod{name, typeType, startLine, startLinePosition, stopLine, stopLinePosition, nil, nil, false, nil}

	updateMethod(method)
}

func (s *JavaCallListener) EnterFormalParameter(ctx *parser.FormalParameterContext) {
	formalParameters[ctx.VariableDeclaratorId().GetText()] = ctx.TypeType().GetText()
}

func (s *JavaCallListener) EnterFieldDeclaration(ctx *parser.FieldDeclarationContext) {
	decelerators := ctx.VariableDeclarators()
	typeType := decelerators.GetParent().GetChild(0).(antlr.ParseTree).GetText()
	for _, declarator := range decelerators.(*parser.VariableDeclaratorsContext).AllVariableDeclarator() {
		value := declarator.(*parser.VariableDeclaratorContext).VariableDeclaratorId().(*parser.VariableDeclaratorIdContext).IDENTIFIER().GetText()
		mapFields[value] = typeType
		fields = append(fields, *&models.JAppField{Type: typeType, Value: value})
	}
}

func (s *JavaCallListener) EnterLocalVariableDeclaration(ctx *parser.LocalVariableDeclarationContext) {
	typ := ctx.GetChild(0).(antlr.ParseTree).GetText()
	if ctx.GetChild(1) != nil {
		if ctx.GetChild(1).GetChild(0) != nil && ctx.GetChild(1).GetChild(0).GetChild(0) != nil {
			variableName := ctx.GetChild(1).GetChild(0).GetChild(0).(antlr.ParseTree).GetText()
			localVars[variableName] = typ
		}
	}
}

func (s *JavaCallListener) EnterMethodDeclaration(ctx *parser.MethodDeclarationContext) {
	startLine := ctx.GetStart().GetLine()
	startLinePosition := ctx.IDENTIFIER().GetSymbol().GetColumn()
	stopLine := ctx.GetStop().GetLine()
	name := ctx.IDENTIFIER().GetText()
	stopLinePosition := startLinePosition + len(name)

	typeType := ctx.TypeTypeOrVoid().GetText()

	method := &models.JMethod{name, typeType, startLine, startLinePosition, stopLine, stopLinePosition, nil, nil, false, nil}

	if ctx.FormalParameters() != nil {
		if ctx.FormalParameters().GetChild(0) == nil || ctx.FormalParameters().GetText() == "()" || ctx.FormalParameters().GetChild(1) == nil {
			updateMethod(method)
			return
		}

		var methodParams []models.JParameter = nil
		parameterList := ctx.FormalParameters().GetChild(1).(*parser.FormalParameterListContext)
		formalParameter := parameterList.AllFormalParameter()
		for _, param := range formalParameter {
			paramContext := param.(*parser.FormalParameterContext)
			paramType := paramContext.TypeType().GetText()
			paramValue := paramContext.VariableDeclaratorId().(*parser.VariableDeclaratorIdContext).IDENTIFIER().GetText()

			localVars[paramValue] = paramType
			methodParams = append(methodParams, *&models.JParameter{Name: paramValue, Type: paramType})
		}

		method.Parameters = methodParams
	}

	updateMethod(method)
}

func updateMethod(method *models.JMethod) {
	currentMethod = *method
	methodQueue = append(methodQueue, *method)
	methodMap[getMethodMapName(*method)] = *method
}

func (s *JavaCallListener) ExitMethodDeclaration(ctx *parser.MethodDeclarationContext) {
	if len(methodQueue) < 1 {
		currentMethod = models.NewJMethod()
		return
	}

	if len(methodQueue) <= 2 {
		currentMethod = methodQueue[0]
	} else {
		methodQueue = methodQueue[0 : len(methodQueue)-1]
		currentMethod = models.NewJMethod()
	}
}

// TODO: add inner creator examples
func (s *JavaCallListener) EnterInnerCreator(ctx *parser.InnerCreatorContext) {
	currentClz = ctx.IDENTIFIER().GetText()
	classQueue = append(classQueue, currentClz)
}

// TODO: add inner creator examples
func (s *JavaCallListener) ExitInnerCreator(ctx *parser.InnerCreatorContext) {
	if len(classQueue) <= 1 {
		return
	}

	classQueue = classQueue[0 : len(classQueue)-1]
	currentClz = classQueue[len(classQueue)]
}

func getMethodMapName(method models.JMethod) string {
	name := method.Name
	if name == "" && len(methodQueue) > 1 {
		name = methodQueue[len(methodQueue)-1].Name
	}
	return currentPkg + "." + currentClz + "." + name
}

func (s *JavaCallListener) EnterCreator(ctx *parser.CreatorContext) {
	variableName := ctx.GetParent().GetParent().GetChild(0).(antlr.ParseTree).GetText()
	createdName := ctx.CreatedName().GetText()
	localVars[variableName] = createdName

	if currentMethod.Name == "" {
		return
	}

	buildCreatedCall(createdName, ctx)
}

func buildCreatedCall(createdName string, ctx *parser.CreatorContext) {
	method := methodMap[getMethodMapName(currentMethod)]
	fullType, _ := warpTargetFullType(createdName)

	startLine := ctx.GetStart().GetLine()
	startLinePosition := ctx.GetStart().GetColumn()
	stopLine := ctx.GetStop().GetLine()
	stopLinePosition := ctx.GetStop().GetColumn()

	jMethodCall := &models.JMethodCall{
		Package:           removeTarget(fullType),
		Type:              "creator",
		Class:             createdName,
		MethodName:        "",
		StartLine:         startLine,
		StartLinePosition: startLinePosition,
		StopLine:          stopLine,
		StopLinePosition:  stopLinePosition,
	}

	method.MethodCalls = append(method.MethodCalls, *jMethodCall)
	methodMap[getMethodMapName(currentMethod)] = method
}

func (s *JavaCallListener) EnterLocalTypeDeclaration(ctx *parser.LocalTypeDeclarationContext) {
	// TODO
}

func (s *JavaCallListener) EnterMethodCall(ctx *parser.MethodCallContext) {
	var jMethodCall = models.NewJMethodCall()

	var targetCtx = ctx.GetParent().GetChild(0).(antlr.ParseTree).GetText()
	var targetType = parseTargetType(targetCtx)
	callee := ctx.GetChild(0).(antlr.ParseTree).GetText()

	jMethodCall.StartLine = ctx.GetStart().GetLine()
	jMethodCall.StartLinePosition = ctx.GetStart().GetColumn()
	jMethodCall.StopLine = ctx.GetStop().GetLine()
	jMethodCall.StopLinePosition = jMethodCall.StartLinePosition + len(callee)

	fullType, callType := warpTargetFullType(targetType)
	if targetType == "super" {
		callType = "super"
		targetType = currentClzExtend
	}
	jMethodCall.Type = callType

	methodName := ctx.IDENTIFIER().GetText()
	packageName := currentPkg

	if fullType != "" {
		if targetType == "" {
			// 处理自调用
			targetType = currentClz
		}

		packageName = removeTarget(fullType)
		methodName = callee
	} else {
		if ctx.GetText() == targetType {
			clz := currentClz
			// 处理 static 方法，如 now()
			for _, imp := range imports {
				if strings.HasSuffix(imp, "."+methodName) {
					packageName = imp
					clz = ""
				}
			}

			targetType = clz
		} else {
			targetType = buildSpecificTarget(targetType)
			targetType = buildMethodNameForBuilder(ctx, targetType)
		}
	}

	jMethodCall.Package = packageName
	jMethodCall.MethodName = methodName

	// TODO: 处理链试调用
	if strings.Contains(targetType, "()") && strings.Contains(targetType, ".") {
		split := strings.Split(targetType, ".")
		targetType = split[0]
	}
	jMethodCall.Class = targetType

	methodCalls = append(methodCalls, jMethodCall)

	method := methodMap[getMethodMapName(currentMethod)]
	method.MethodCalls = append(method.MethodCalls, jMethodCall)
	methodMap[getMethodMapName(currentMethod)] = method
}

func isChainCall(targetType string) bool {
	return strings.Contains(targetType, "(") && strings.Contains(targetType, ")") && strings.Contains(targetType, ".")
}

func buildMethodNameForBuilder(ctx *parser.MethodCallContext, targetType string) string {
	// TODO: refactor
	if reflect.TypeOf(ctx.GetParent()).String() == "*parser.ExpressionContext" {
		parentCtx := ctx.GetParent().(*parser.ExpressionContext)
		if reflect.TypeOf(parentCtx.GetParent()).String() == "*parser.VariableInitializerContext" {
			varParent := parentCtx.GetParent().(*parser.VariableInitializerContext).GetParent()
			if reflect.TypeOf(varParent).String() == "*parser.VariableDeclaratorContext" {
				varDeclParent := varParent.(*parser.VariableDeclaratorContext).GetParent()
				if reflect.TypeOf(varDeclParent).String() == "*parser.VariableDeclaratorsContext" {
					parent := varDeclParent.(*parser.VariableDeclaratorsContext).GetParent()
					if reflect.TypeOf(parent).String() == "*parser.LocalVariableDeclarationContext" {
						context := parent.(*parser.LocalVariableDeclarationContext)
						targetType = context.TypeType().GetText()
					}
				}
			}
		}
	}

	return targetType
}

func buildSpecificTarget(targetType string) string {
	isSelfFieldCall := strings.Contains(targetType, "this.")
	if isSelfFieldCall {
		targetType = strings.ReplaceAll(targetType, "this.", "")
		for _, field := range fields {
			if field.Value == targetType {
				targetType = field.Type
			}
		}
	}
	return targetType
}

func (s *JavaCallListener) EnterExpression(ctx *parser.ExpressionContext) {
	// lambda BlogPO::of
	if ctx.COLONCOLON() != nil {
		if ctx.Expression(0) == nil {
			return
		}

		text := ctx.Expression(0).GetText()
		methodName := ctx.IDENTIFIER().GetText()
		targetType := parseTargetType(text)

		fullType, _ := warpTargetFullType(targetType)

		startLine := ctx.GetStart().GetLine()
		startLinePosition := ctx.GetStart().GetColumn()
		stopLine := ctx.GetStop().GetLine()
		stopLinePosition := startLinePosition + len(text)

		jMethodCall := &models.JMethodCall{removeTarget(fullType), "lambda", targetType, methodName, startLine, startLinePosition, stopLine, stopLinePosition}
		methodCalls = append(methodCalls, *jMethodCall)
	}
}

func (s *JavaCallListener) appendClasses(classes []string) {
	clzs = classes
}

func removeTarget(fullType string) string {
	split := strings.Split(fullType, ".")
	return strings.Join(split[:len(split)-1], ".")
}

func parseTargetType(targetCtx string) string {
	targetVar := targetCtx
	targetType := targetVar

	//TODO: update this reflect
	typeOf := reflect.TypeOf(targetCtx).String()
	if strings.HasSuffix(typeOf, "MethodCallContext") {
		targetType = currentClz
	} else {
		//if isChainCall(targetVar) {
		//	split := strings.Split(targetType, ".")
		//	targetVar = split[0]
		//}

		fieldType := mapFields[targetVar]
		formalType := formalParameters[targetVar]
		localVarType := localVars[targetVar]
		if fieldType != "" {
			targetType = fieldType
		} else if formalType != "" {
			targetType = formalType
		} else if localVarType != "" {
			targetType = localVarType
		}
	}

	return targetType
}

func warpTargetFullType(targetType string) (string, string) {
	callType := ""
	if strings.EqualFold(currentClz, targetType) {
		callType = "self"
		return currentPkg + "." + targetType, ""
	}

	// TODO: update for array
	split := strings.Split(targetType, ".")
	str := split[0]
	pureTargetType := strings.ReplaceAll(strings.ReplaceAll(str, "[", ""), "]", "")

	if pureTargetType != "" {
		for _, imp := range imports {
			if strings.HasSuffix(imp, pureTargetType) {
				callType = "chain"
				return imp, callType
			}
		}
	}

	//maybe the same package
	for _, clz := range clzs {
		if strings.HasSuffix(clz, "."+pureTargetType) {
			callType = "same package"
			return clz, callType
		}
	}

	//1. current package, 2. import by *
	if pureTargetType == "super" {
		for _, imp := range imports {
			if strings.HasSuffix(imp, currentClzExtend) {
				callType = "super"
				return imp, callType
			}
		}
	}

	if _, ok := identMap[currentPkg+"."+targetType]; ok {
		callType = "same package 2"
		return currentPkg + "." + targetType, callType
	}

	return "", callType
}
