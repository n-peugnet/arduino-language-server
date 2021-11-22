package ls

import (
	"github.com/arduino/arduino-language-server/sourcemapper"
	"go.bug.st/lsp"
	"go.bug.st/lsp/jsonrpc"
)

func (ls *INOLanguageServer) idePathToIdeURI(logger jsonrpc.FunctionLogger, inoPath string) (lsp.DocumentURI, error) {
	if inoPath == sourcemapper.NotIno.File {
		return sourcemapper.NotInoURI, nil
	}
	doc, ok := ls.trackedIDEDocs[inoPath]
	if !ok {
		logger.Logf("    !!! Unresolved .ino path: %s", inoPath)
		logger.Logf("    !!! Known doc paths are:")
		for p := range ls.trackedIDEDocs {
			logger.Logf("    !!! > %s", p)
		}
		uri := lsp.NewDocumentURI(inoPath)
		return uri, &UnknownURI{uri}
	}
	return doc.URI, nil
}

func (ls *INOLanguageServer) ide2ClangTextDocumentIdentifier(logger jsonrpc.FunctionLogger, ideTextDocIdentifier lsp.TextDocumentIdentifier) (lsp.TextDocumentIdentifier, error) {
	clangURI, err := ls.ide2ClangDocumentURI(logger, ideTextDocIdentifier.URI)
	return lsp.TextDocumentIdentifier{URI: clangURI}, err
}

func (ls *INOLanguageServer) ide2ClangDocumentURI(logger jsonrpc.FunctionLogger, ideURI lsp.DocumentURI) (lsp.DocumentURI, error) {
	// Sketchbook/Sketch/Sketch.ino      -> build-path/sketch/Sketch.ino.cpp
	// Sketchbook/Sketch/AnotherTab.ino  -> build-path/sketch/Sketch.ino.cpp  (different section from above)
	idePath := ideURI.AsPath()
	if idePath.Ext() == ".ino" {
		clangURI := lsp.NewDocumentURIFromPath(ls.buildSketchCpp)
		logger.Logf("URI: %s -> %s", ideURI, clangURI)
		return clangURI, nil
	}

	// another/path/source.cpp -> another/path/source.cpp (unchanged)
	inside, err := idePath.IsInsideDir(ls.sketchRoot)
	if err != nil {
		logger.Logf("ERROR: could not determine if '%s' is inside '%s'", idePath, ls.sketchRoot)
		return lsp.NilURI, &UnknownURI{ideURI}
	}
	if !inside {
		clangURI := ideURI
		logger.Logf("URI: %s -> %s", ideURI, clangURI)
		return clangURI, nil
	}

	// Sketchbook/Sketch/AnotherFile.cpp -> build-path/sketch/AnotherFile.cpp
	rel, err := ls.sketchRoot.RelTo(idePath)
	if err != nil {
		logger.Logf("ERROR: could not determine rel-path of '%s' in '%s': %s", idePath, ls.sketchRoot, err)
		return lsp.NilURI, err
	}

	clangPath := ls.buildSketchRoot.JoinPath(rel)
	clangURI := lsp.NewDocumentURIFromPath(clangPath)
	logger.Logf("URI: %s -> %s", ideURI, clangURI)
	return clangURI, nil
}

func (ls *INOLanguageServer) ide2ClangTextDocumentPositionParams(logger jsonrpc.FunctionLogger, ideParams lsp.TextDocumentPositionParams) (lsp.TextDocumentPositionParams, error) {
	ideTextDocument := ideParams.TextDocument
	idePosition := ideParams.Position
	ideURI := ideTextDocument.URI

	clangTextDocument, err := ls.ide2ClangTextDocumentIdentifier(logger, ideTextDocument)
	if err != nil {
		logger.Logf("%s -> invalid text document: %s", ideParams, err)
		return lsp.TextDocumentPositionParams{}, err
	}
	clangPosition := idePosition
	if ls.clangURIRefersToIno(clangTextDocument.URI) {
		if cppLine, ok := ls.sketchMapper.InoToCppLineOk(ideURI, idePosition.Line); ok {
			clangPosition.Line = cppLine
		} else {
			logger.Logf("%s -> invalid line requested: %s:%d", ideParams, ideURI, idePosition.Line)
			return lsp.TextDocumentPositionParams{}, &UnknownURI{ideURI}
		}
	}
	clangParams := lsp.TextDocumentPositionParams{
		TextDocument: clangTextDocument,
		Position:     clangPosition,
	}
	logger.Logf("%s -> %s", ideParams, clangParams)
	return clangParams, nil
}