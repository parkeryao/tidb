// Copyright 2015 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package plan

import (
	"fmt"
	"testing"

	. "github.com/pingcap/check"
	"github.com/pingcap/tidb/ast"
	"github.com/pingcap/tidb/model"
	"github.com/pingcap/tidb/mysql"
	"github.com/pingcap/tidb/parser"
)

var _ = Suite(&testPlanSuite{})

func TestT(t *testing.T) {
	TestingT(t)
}

type testPlanSuite struct{}

func (s *testPlanSuite) TestRangeBuilder(c *C) {
	rb := &rangeBuilder{}

	cases := []struct {
		exprStr   string
		resultStr string
	}{
		{
			exprStr:   "a = 1",
			resultStr: "[[1 1]]",
		},
		{
			exprStr:   "1 = a",
			resultStr: "[[1 1]]",
		},
		{
			exprStr:   "a != 1",
			resultStr: "[[-inf 1) (1 +inf]]",
		},
		{
			exprStr:   "1 != a",
			resultStr: "[[-inf 1) (1 +inf]]",
		},
		{
			exprStr:   "a > 1",
			resultStr: "[(1 +inf]]",
		},
		{
			exprStr:   "1 < a",
			resultStr: "[(1 +inf]]",
		},
		{
			exprStr:   "a >= 1",
			resultStr: "[[1 +inf]]",
		},
		{
			exprStr:   "1 <= a",
			resultStr: "[[1 +inf]]",
		},
		{
			exprStr:   "a < 1",
			resultStr: "[[-inf 1)]",
		},
		{
			exprStr:   "1 > a",
			resultStr: "[[-inf 1)]",
		},
		{
			exprStr:   "a <= 1",
			resultStr: "[[-inf 1]]",
		},
		{
			exprStr:   "1 >= a",
			resultStr: "[[-inf 1]]",
		},
		{
			exprStr:   "(a)",
			resultStr: "[[-inf 0) (0 +inf]]",
		},
		{
			exprStr:   "a in (1, 3, NULL, 2)",
			resultStr: "[[<nil> <nil>] [1 1] [2 2] [3 3]]",
		},
		{
			exprStr:   "a between 1 and 2",
			resultStr: "[[1 2]]",
		},
		{
			exprStr:   "a not between 1 and 2",
			resultStr: "[[-inf 1) (2 +inf]]",
		},
		{
			exprStr:   "a not between null and 0",
			resultStr: "[(0 +inf]]",
		},
		{
			exprStr:   "a between 2 and 1",
			resultStr: "[]",
		},
		{
			exprStr:   "a not between 2 and 1",
			resultStr: "[[-inf +inf]]",
		},
		{
			exprStr:   "a IS NULL",
			resultStr: "[[<nil> <nil>]]",
		},
		{
			exprStr:   "a IS NOT NULL",
			resultStr: "[[-inf +inf]]",
		},
		{
			exprStr:   "a IS TRUE",
			resultStr: "[[-inf 0) (0 +inf]]",
		},
		{
			exprStr:   "a IS NOT TRUE",
			resultStr: "[[<nil> <nil>] [0 0]]",
		},
		{
			exprStr:   "a IS FALSE",
			resultStr: "[[0 0]]",
		},
		{
			exprStr:   "a IS NOT FALSE",
			resultStr: "[[<nil> 0) (0 +inf]]",
		},
		{
			exprStr:   "a LIKE 'abc%'",
			resultStr: "[[abc abd)]",
		},
		{
			exprStr:   "a LIKE 'abc_'",
			resultStr: "[(abc abd)]",
		},
		{
			exprStr:   "a LIKE '%'",
			resultStr: "[[-inf +inf]]",
		},
		{
			exprStr:   `a LIKE '\%a'`,
			resultStr: `[[%a %b)]`,
		},
		{
			exprStr:   `a LIKE "\\"`,
			resultStr: `[[\ ])]`,
		},
		{
			exprStr:   `a LIKE "\\\\a%"`,
			resultStr: `[[\a \b)]`,
		},
		{
			exprStr:   `a > 0 AND a < 1`,
			resultStr: `[(0 1)]`,
		},
		{
			exprStr:   `a > 1 AND a < 0`,
			resultStr: `[]`,
		},
		{
			exprStr:   `a > 1 OR a < 0`,
			resultStr: `[[-inf 0) (1 +inf]]`,
		},
		{
			exprStr:   `(a > 1 AND a < 2) OR (a > 3 AND a < 4)`,
			resultStr: `[(1 2) (3 4)]`,
		},
		{
			exprStr:   `(a < 0 OR a > 3) AND (a < 1 OR a > 4)`,
			resultStr: `[[-inf 0) (4 +inf]]`,
		},
		{
			exprStr:   `a > NULL`,
			resultStr: `[]`,
		},
	}

	for _, ca := range cases {
		sql := "select 1 from dual where " + ca.exprStr
		stmts, err := parser.Parse(sql, "", "")
		c.Assert(err, IsNil, Commentf("error %v, for expr %s", err, ca.exprStr))
		stmt := stmts[0].(*ast.SelectStmt)
		result := rb.build(stmt.Where)
		c.Assert(rb.err, IsNil)
		got := fmt.Sprintf("%v", result)
		c.Assert(got, Equals, ca.resultStr, Commentf("differen for expr %s", ca.exprStr))
	}
}

func (s *testPlanSuite) TestFilterRate(c *C) {
	cases := []struct {
		expr string
		rate float64
	}{
		{expr: "a = 1", rate: rateEqual},
		{expr: "a > 1", rate: rateGreaterOrLess},
		{expr: "a between 1 and 100", rate: rateBetween},
		{expr: "a is null", rate: rateIsNull},
		{expr: "a is not null", rate: rateFull - rateIsNull},
		{expr: "a is true", rate: rateFull - rateIsNull - rateIsFalse},
		{expr: "a is not true", rate: rateIsNull + rateIsFalse},
		{expr: "a is false", rate: rateIsFalse},
		{expr: "a is not false", rate: rateFull - rateIsFalse},
		{expr: "a like 'a'", rate: rateLike},
		{expr: "a not like 'a'", rate: rateFull - rateLike},
		{expr: "a in (1, 2, 3)", rate: rateEqual * 3},
		{expr: "a not in (1, 2, 3)", rate: rateFull - rateEqual*3},
		{expr: "a > 1 and a < 9", rate: float64(rateGreaterOrLess) * float64(rateGreaterOrLess)},
		{expr: "a = 1 or a = 2", rate: rateEqual + rateEqual - rateEqual*rateEqual},
		{expr: "a != 1", rate: rateNotEqual},
	}
	for _, ca := range cases {
		sql := "select 1 from dual where " + ca.expr
		s, err := parser.ParseOneStmt(sql, "", "")
		c.Assert(err, IsNil, Commentf("for expr %s", ca.expr))
		stmt := s.(*ast.SelectStmt)
		rate := guesstimateFilterRate(stmt.Where)
		c.Assert(rate, Equals, ca.rate, Commentf("for expr %s", ca.expr))
	}
}

func (s *testPlanSuite) TestBestPlan(c *C) {
	cases := []struct {
		sql  string
		best string
	}{
		{
			sql:  "select * from t",
			best: "Table(t)->Fields",
		},
		{
			sql:  "select * from t order by a",
			best: "Table(t)->Fields",
		},
		{
			sql:  "select * from t where b = 1 order by a",
			best: "Index(t.b)->Fields->Sort",
		},
		{
			sql:  "select * from t where (a between 1 and 2) and (b = 3)",
			best: "Index(t.b)->Fields",
		},
		{
			sql:  "select * from t where a > 0 order by b limit 100",
			best: "Index(t.b)->Fields->Limit",
		},
		{
			sql:  "select * from t where d = 0",
			best: "Table(t)->Fields",
		},
		{
			sql:  "select * from t where c = 0 and d = 0",
			best: "Index(t.c_d)->Fields",
		},
		{
			sql:  "select * from t where b like 'abc%'",
			best: "Index(t.b)->Fields",
		},
		{
			sql:  "select * from t where d",
			best: "Table(t)->Fields",
		},
		{
			sql:  "select * from t where a is null",
			best: "Range(t)->Fields",
		},
		{
			sql:  "select a from t where a = 1 limit 1 for update",
			best: "Range(t)->Lock->Fields->Limit",
		},
		{
			sql:  "admin show ddl",
			best: "ShowDDL",
		},
		{
			sql:  "admin check table t",
			best: "CheckTable",
		},
	}
	for _, ca := range cases {
		comment := Commentf("for %s", ca.sql)
		stmt, err := parser.ParseOneStmt(ca.sql, "", "")
		c.Assert(err, IsNil, comment)
		ast.SetFlag(stmt)
		mockResolve(stmt)

		p, err := BuildPlan(stmt)
		c.Assert(err, IsNil)

		err = Refine(p)
		explainStr, err := Explain(p)
		c.Assert(err, IsNil)
		c.Assert(explainStr, Equals, ca.best, Commentf("for %s cost %v", ca.sql, EstimateCost(p)))
	}
}

func (s *testPlanSuite) TestSplitWhere(c *C) {
	cases := []struct {
		expr  string
		count int
	}{
		{"a = 1 and b = 2 and c = 3", 3},
		{"(a = 1 and b = 2) and c = 3", 3},
		{"a = 1 and (b = 2 and c = 3 or d = 4)", 2},
		{"a = 1 and (b = 2 or c = 3) and d = 4", 3},
		{"(a = 1 and b = 2) and (c = 3 and d = 4)", 4},
	}
	for _, ca := range cases {
		sql := "select 1 from dual where " + ca.expr
		comment := Commentf("for expr %s", ca.expr)
		s, err := parser.ParseOneStmt(sql, "", "")
		c.Assert(err, IsNil, comment)
		stmt := s.(*ast.SelectStmt)
		conditions := splitWhere(stmt.Where)
		c.Assert(conditions, HasLen, ca.count, comment)
	}
}

func mockResolve(node ast.Node) {
	indices := []*model.IndexInfo{
		{
			Name: model.NewCIStr("b"),
			Columns: []*model.IndexColumn{
				{
					Name: model.NewCIStr("b"),
				},
			},
		},
		{
			Name: model.NewCIStr("c_d"),
			Columns: []*model.IndexColumn{
				{
					Name: model.NewCIStr("c"),
				},
				{
					Name: model.NewCIStr("d"),
				},
			},
		},
	}
	pkColumn := &model.ColumnInfo{
		Name: model.NewCIStr("a"),
	}
	pkColumn.Flag = mysql.PriKeyFlag
	table := &model.TableInfo{
		Columns:    []*model.ColumnInfo{pkColumn},
		Indices:    indices,
		Name:       model.NewCIStr("t"),
		PKIsHandle: true,
	}
	resolver := mockResolver{table: table}
	node.Accept(&resolver)
}

type mockResolver struct {
	table *model.TableInfo
}

func (b *mockResolver) Enter(in ast.Node) (ast.Node, bool) {
	return in, false
}

func (b *mockResolver) Leave(in ast.Node) (ast.Node, bool) {
	switch x := in.(type) {
	case *ast.ColumnNameExpr:
		x.Refer = &ast.ResultField{
			Column: &model.ColumnInfo{
				Name: x.Name.Name,
			},
			Table: b.table,
		}
		if x.Name.Name.L == "a" {
			x.Refer.Column = b.table.Columns[0]
		}
	case *ast.TableName:
		x.TableInfo = b.table
	}
	return in, true
}
