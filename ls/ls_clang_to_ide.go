package ls

import (
	"github.com/arduino/arduino-language-server/sourcemapper"
	"go.bug.st/lsp"
	"go.bug.st/lsp/jsonrpc"
)

func (ls *INOLanguageServer) clang2IdeRangeAndDocumentURI(logger jsonrpc.FunctionLogger, clangURI lsp.DocumentURI, clangRange lsp.Range) (lsp.DocumentURI, lsp.Range, bool, error) {
	// Sketchbook/Sketch/Sketch.ino      <-> build-path/sketch/Sketch.ino.cpp
	// Sketchbook/Sketch/AnotherTab.ino  <-> build-path/sketch/Sketch.ino.cpp  (different section from above)
	if ls.clangURIRefersToIno(clangURI) {
		// We are converting from preprocessed sketch.ino.cpp back to a sketch.ino file
		idePath, ideRange, err := ls.sketchMapper.CppToInoRangeOk(clangRange)
		if _, ok := err.(sourcemapper.AdjustedRangeErr); ok {
			logger.Logf("Range has been END LINE ADJSUTED")
		} else if err != nil {
			logger.Logf("Range conversion ERROR: %s", err)
			ls.sketchMapper.DebugLogAll()
			return lsp.NilURI, lsp.NilRange, false, err
		}
		ideURI, err := ls.idePathToIdeURI(logger, idePath)
		if err != nil {
			logger.Logf("Range conversion ERROR: %s", err)
			ls.sketchMapper.DebugLogAll()
			return lsp.NilURI, lsp.NilRange, false, err
		}
		inPreprocessed := ls.sketchMapper.IsPreprocessedCppLine(clangRange.Start.Line)
		if inPreprocessed {
			logger.Logf("Range is in PREPROCESSED section of the sketch")
		}
		logger.Logf("Range: %s:%s -> %s:%s", clangURI, clangRange, ideURI, ideRange)
		return ideURI, ideRange, inPreprocessed, err
	}

	// /another/global/path/to/source.cpp <-> /another/global/path/to/source.cpp (same range)
	ideRange := clangRange
	clangPath := clangURI.AsPath()
	inside, err := clangPath.IsInsideDir(ls.buildSketchRoot)
	if err != nil {
		logger.Logf("ERROR: could not determine if '%s' is inside '%s'", clangURI, ls.buildSketchRoot)
		return lsp.NilURI, lsp.NilRange, false, err
	}
	if !inside {
		ideURI := clangURI
		logger.Logf("Range: %s:%s -> %s:%s", clangURI, clangRange, ideURI, ideRange)
		return clangURI, clangRange, false, nil
	}

	// Sketchbook/Sketch/AnotherFile.cpp <-> build-path/sketch/AnotherFile.cpp (same range)
	rel, err := ls.buildSketchRoot.RelTo(clangPath)
	if err != nil {
		logger.Logf("ERROR: could not transform '%s' into a relative path on '%s': %s", clangURI, ls.buildSketchRoot, err)
		return lsp.NilURI, lsp.NilRange, false, err
	}
	idePath := ls.sketchRoot.JoinPath(rel).String()
	ideURI, err := ls.idePathToIdeURI(logger, idePath)
	logger.Logf("Range: %s:%s -> %s:%s", clangURI, clangRange, ideURI, ideRange)
	return ideURI, clangRange, false, err
}

func (ls *INOLanguageServer) clang2IdeDocumentHighlight(logger jsonrpc.FunctionLogger, clangHighlight lsp.DocumentHighlight, cppURI lsp.DocumentURI) (lsp.DocumentHighlight, bool, error) {
	_, ideRange, inPreprocessed, err := ls.clang2IdeRangeAndDocumentURI(logger, cppURI, clangHighlight.Range)
	if err != nil || inPreprocessed {
		return lsp.DocumentHighlight{}, inPreprocessed, err
	}
	return lsp.DocumentHighlight{
		Kind:  clangHighlight.Kind,
		Range: ideRange,
	}, false, nil
}

func (ls *INOLanguageServer) clang2IdeDiagnostics(logger jsonrpc.FunctionLogger, clangDiagsParams *lsp.PublishDiagnosticsParams) (map[lsp.DocumentURI]*lsp.PublishDiagnosticsParams, error) {
	// If diagnostics comes from sketch.ino.cpp they may refer to multiple .ino files,
	// so we collect all of the into a map.
	allIdeDiagsParams := map[lsp.DocumentURI]*lsp.PublishDiagnosticsParams{}

	for _, clangDiagnostic := range clangDiagsParams.Diagnostics {
		ideURI, ideDiagnostic, inPreprocessed, err := ls.clang2IdeDiagnostic(logger, clangDiagsParams.URI, clangDiagnostic)
		if err != nil {
			return nil, err
		}
		if inPreprocessed {
			continue
		}
		if _, ok := allIdeDiagsParams[ideURI]; !ok {
			allIdeDiagsParams[ideURI] = &lsp.PublishDiagnosticsParams{URI: ideURI}
		}
		allIdeDiagsParams[ideURI].Diagnostics = append(allIdeDiagsParams[ideURI].Diagnostics, ideDiagnostic)
	}

	return allIdeDiagsParams, nil
}

func (ls *INOLanguageServer) clang2IdeDiagnostic(logger jsonrpc.FunctionLogger, clangURI lsp.DocumentURI, clangDiagnostic lsp.Diagnostic) (lsp.DocumentURI, lsp.Diagnostic, bool, error) {
	ideURI, ideRange, inPreproccesed, err := ls.clang2IdeRangeAndDocumentURI(logger, clangURI, clangDiagnostic.Range)
	if err != nil || inPreproccesed {
		return lsp.DocumentURI{}, lsp.Diagnostic{}, inPreproccesed, err
	}

	ideDiagnostic := clangDiagnostic
	ideDiagnostic.Range = ideRange

	if len(clangDiagnostic.RelatedInformation) > 0 {
		ideInfos, err := ls.clang2IdeDiagnosticRelatedInformationArray(logger, clangDiagnostic.RelatedInformation)
		if err != nil {
			return lsp.DocumentURI{}, lsp.Diagnostic{}, false, err
		}
		ideDiagnostic.RelatedInformation = ideInfos
	}
	return ideURI, ideDiagnostic, false, nil
}

func (ls *INOLanguageServer) clang2IdeDiagnosticRelatedInformationArray(logger jsonrpc.FunctionLogger, clangInfos []lsp.DiagnosticRelatedInformation) ([]lsp.DiagnosticRelatedInformation, error) {
	ideInfos := []lsp.DiagnosticRelatedInformation{}
	for _, clangInfo := range clangInfos {
		ideLocation, inPreprocessed, err := ls.cpp2inoLocation(logger, clangInfo.Location)
		if err != nil {
			return nil, err
		}
		if inPreprocessed {
			logger.Logf("Ignoring in-preprocessed-section diagnostic related information")
			continue
		}
		ideInfos = append(ideInfos, lsp.DiagnosticRelatedInformation{
			Message:  clangInfo.Message,
			Location: ideLocation,
		})
	}
	return ideInfos, nil
}

func (ls *INOLanguageServer) clang2IdeSymbolInformation(clangSymbolsInformation []lsp.SymbolInformation) []lsp.SymbolInformation {
	panic("not implemented")
}