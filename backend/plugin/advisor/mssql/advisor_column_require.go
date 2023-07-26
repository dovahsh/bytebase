// Package mssql is the advisor for MSSQL database.
package mssql

import (
	"fmt"

	"github.com/antlr4-go/antlr/v4"
	parser "github.com/bytebase/tsql-parser"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"

	"github.com/bytebase/bytebase/backend/plugin/advisor"
	"github.com/bytebase/bytebase/backend/plugin/advisor/db"
	bbparser "github.com/bytebase/bytebase/backend/plugin/parser/sql"
)

var (
	_ advisor.Advisor = (*ColumnRequireAdvisor)(nil)
)

func init() {
	advisor.Register(db.MSSQL, advisor.MSSQLColumnRequirement, &ColumnRequireAdvisor{})
}

// ColumnRequireAdvisor is the advisor checking for column requirement..
type ColumnRequireAdvisor struct {
}

// Check checks for column requirement..
func (*ColumnRequireAdvisor) Check(ctx advisor.Context, _ string) ([]advisor.Advice, error) {
	tree, ok := ctx.AST.(antlr.Tree)
	if !ok {
		return nil, errors.Errorf("failed to convert to Tree")
	}

	level, err := advisor.NewStatusBySQLReviewRuleLevel(ctx.Rule.Level)
	if err != nil {
		return nil, err
	}

	columnList, err := advisor.UnmarshalRequiredColumnList(ctx.Rule.Payload)
	if err != nil {
		return nil, err
	}

	listener := &columnRequireChecker{
		level:          level,
		title:          string(ctx.Rule.Type),
		requireColumns: make(map[string]any),
	}

	for _, column := range columnList {
		listener.requireColumns[column] = true
	}

	antlr.ParseTreeWalkerDefault.Walk(listener, tree)

	return listener.generateAdvice()
}

// columnRequireChecker is the listener for column requirement.
type columnRequireChecker struct {
	*parser.BaseTSqlParserListener

	level advisor.Status
	title string

	adviceList []advisor.Advice

	// requireColumns is the required columns, the key is the normalized column name.
	requireColumns map[string]any

	// The following variables should be clean up when ENTER some statement.
	//
	// currentMissingColumn is the missing column, the key is the normalized column name.
	currentMissingColumn map[string]any
	// currentOriginalTableName is the original table name, should be reset when QUIT some statement.
	currentOriginalTableName string
}

// generateAdvice returns the advices generated by the listener, the advices must not be empty.
func (l *columnRequireChecker) generateAdvice() ([]advisor.Advice, error) {
	if len(l.adviceList) == 0 {
		l.adviceList = append(l.adviceList, advisor.Advice{
			Status:  advisor.Success,
			Code:    advisor.Ok,
			Title:   "OK",
			Content: "",
		})
	}
	return l.adviceList, nil
}

func (l *columnRequireChecker) EnterCreate_table(ctx *parser.Create_tableContext) {
	l.currentOriginalTableName = ctx.Table_name().GetText()
	l.currentMissingColumn = make(map[string]any)
	for column := range l.requireColumns {
		l.currentMissingColumn[column] = true
	}
}

func (l *columnRequireChecker) EnterColumn_definition(ctx *parser.Column_definitionContext) {
	if l.currentOriginalTableName == "" {
		return
	}

	normalizedColumnName := bbparser.NormalizeTSQLIdentifier(ctx.Id_())
	delete(l.currentMissingColumn, normalizedColumnName)
}

func (l *columnRequireChecker) ExitCreate_table(ctx *parser.Create_tableContext) {
	columnNames := make([]string, 0, len(l.currentMissingColumn))
	for column := range l.currentMissingColumn {
		columnNames = append(columnNames, column)
	}
	if len(columnNames) == 0 {
		return
	}

	slices.SortFunc[string](columnNames, func(i, j string) bool {
		return i < j
	})
	for _, column := range columnNames {
		l.adviceList = append(l.adviceList, advisor.Advice{
			Status:  l.level,
			Code:    advisor.NoRequiredColumn,
			Title:   l.title,
			Content: fmt.Sprintf("Table %s missing required column %q", l.currentOriginalTableName, column),
			Line:    ctx.GetStart().GetLine(),
		})
	}

	l.currentOriginalTableName = ""
	l.currentMissingColumn = nil
}

func (l *columnRequireChecker) EnterAlter_table(ctx *parser.Alter_tableContext) {
	if ctx.DROP() == nil || ctx.COLUMN() == nil {
		return
	}

	tableName := ctx.Table_name(0).GetText()
	allColumnNames := ctx.AllId_()
	for _, columnName := range allColumnNames {
		normalizedColumnName := bbparser.NormalizeTSQLIdentifier(columnName)
		if _, ok := l.requireColumns[normalizedColumnName]; ok {
			l.adviceList = append(l.adviceList, advisor.Advice{
				Status:  l.level,
				Code:    advisor.NoRequiredColumn,
				Title:   l.title,
				Content: fmt.Sprintf("Table %s missing required column %q", tableName, normalizedColumnName),
				Line:    ctx.GetStart().GetLine(),
			})
		}
	}
}