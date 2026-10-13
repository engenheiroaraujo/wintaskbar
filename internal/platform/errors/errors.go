// Package errs é o catálogo de erros: o conjunto fechado de categorias, todas
// as sentinelas nomeadas do projeto e o mapeamento de categoria para o código
// de saída do processo.
//
// As mensagens de erros categorizados são lidas pelo usuário, então ficam no
// idioma do produto (português). As causas embrulhadas nelas ficam em inglês,
// como os logs.
package errs

import (
	"errors"
	"fmt"
	"strings"
)

// Category classifica um erro; ela decide o código de saída.
type Category string

const (
	// InvalidInput: comando desconhecido, argumento ou opção inválidos.
	InvalidInput Category = "invalid_input"
	// InvalidTheme: o arquivo do Theme tem erros (todos em Details).
	InvalidTheme Category = "invalid_theme"
	// Unavailable: o explorer não está rodando, ou a anexação do TAP falhou.
	Unavailable Category = "unavailable"
)

// Códigos de saída por categoria; erros sem categoria saem com 1.
var exitCodes = map[Category]int{
	InvalidInput: 2,
	InvalidTheme: 3,
	Unavailable:  4,
}

// Detail é um problema localizado, por exemplo um erro em um arquivo de Theme.
type Detail struct {
	Where   string
	Message string
}

// Error é um erro categorizado.
type Error struct {
	Category Category
	Message  string
	Details  []Detail
	Err      error
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(e.Message)
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

func (e *Error) Unwrap() error { return e.Err }

// New devolve um erro categorizado.
func New(category Category, message string) *Error {
	return &Error{Category: category, Message: message}
}

// Wrap devolve um erro categorizado em volta de uma causa.
func Wrap(category Category, message string, err error) *Error {
	return &Error{Category: category, Message: message, Err: err}
}

// WithDetails devolve um erro categorizado com detalhes localizados.
func WithDetails(category Category, message string, details []Detail) *Error {
	return &Error{Category: category, Message: message, Details: details}
}

// PreserveOr mantém a categoria de um erro já categorizado, ou embrulha err
// com category quando ele ainda não tem uma.
func PreserveOr(category Category, message string, err error) error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return fmt.Errorf("%s: %w", message, err)
	}
	return Wrap(category, message, err)
}

// CategoryOf devolve a categoria de err, ou "" quando ele não tem uma.
func CategoryOf(err error) Category {
	var e *Error
	if errors.As(err, &e) {
		return e.Category
	}
	return ""
}

// DetailsOf devolve os detalhes de err, se houver.
func DetailsOf(err error) []Detail {
	var e *Error
	if errors.As(err, &e) {
		return e.Details
	}
	return nil
}

// ExitCode mapeia err para o código de saída do processo: 0 quando nulo, o
// código da categoria quando categorizado, 1 nos demais casos.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if code, ok := exitCodes[CategoryOf(err)]; ok {
		return code
	}
	return 1
}

// Sentinelas. Todo erro nomeado do projeto vive aqui.
var (
	// ErrNoTaskbar: nenhum explorer possui a Taskbar (ainda não subiu, ou está reiniciando).
	ErrNoTaskbar = New(Unavailable, "a barra de tarefas não foi encontrada (o explorer está em execução?)")

	// ErrNoDiagConnection: nenhum ponto do diagnóstico XAML aceitou a anexação.
	ErrNoDiagConnection = New(Unavailable, "o explorer não aceitou o diagnóstico XAML")
)
