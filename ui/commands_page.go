package ui

import (
	"html"
	"html/template"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/auvitly/cmdui.git/internal/domain"
)

const labelRowHTML = `{{define "label-row"}}
<div class="label-row">
  <div class="field"><label>Ключ</label><input name="label_key" maxlength="80" placeholder="environment" value="{{.Key}}"></div>
  <div class="field"><label>Значение</label><input name="label_value" maxlength="120" placeholder="production" value="{{.Value}}"></div>
  <input type="hidden" name="label_color" value="{{if .Color}}{{.Color}}{{else}}#DCEBFA{{end}}">
  <button class="color-trigger" type="button" aria-label="Выбрать цвет label" title="Цвет label"><span class="color-dot" style="background:{{if .Color}}{{.Color}}{{else}}#DCEBFA{{end}}"></span></button>
  <button class="button icon-button remove-label" type="button" aria-label="Удалить label" title="Удалить label">×</button>
  <div class="color-popover" hidden>
    <div class="popover-head"><strong>Цвет label</strong><button class="popover-close" type="button" aria-label="Закрыть">×</button></div>
    <div class="swatches" aria-label="Пастельная палитра">
      <button class="swatch" type="button" data-palette-color="#DCEBFA" style="background:#DCEBFA" aria-label="Голубой пастельный"></button>
      <button class="swatch" type="button" data-palette-color="#DDF2E1" style="background:#DDF2E1" aria-label="Зеленый пастельный"></button>
      <button class="swatch" type="button" data-palette-color="#FCE8D5" style="background:#FCE8D5" aria-label="Персиковый пастельный"></button>
      <button class="swatch" type="button" data-palette-color="#F7DDE3" style="background:#F7DDE3" aria-label="Розовый пастельный"></button>
      <button class="swatch" type="button" data-palette-color="#E9DFF5" style="background:#E9DFF5" aria-label="Сиреневый пастельный"></button>
      <button class="swatch" type="button" data-palette-color="#F5F0CF" style="background:#F5F0CF" aria-label="Желтый пастельный"></button>
      <button class="swatch" type="button" data-palette-color="#D8EFEE" style="background:#D8EFEE" aria-label="Бирюзовый пастельный"></button>
      <button class="swatch" type="button" data-palette-color="#E8E4DC" style="background:#E8E4DC" aria-label="Серо-бежевый пастельный"></button>
    </div>
    <div class="custom-color"><label>Любой цвет <input class="color-picker" type="color" value="{{if .Color}}{{.Color}}{{else}}#DCEBFA{{end}}" aria-label="Произвольный цвет label"></label></div>
    <div class="rgb-controls">
      <label class="rgb">R <output>220</output><input data-rgb="R" type="range" min="0" max="255" value="220"></label>
      <label class="rgb">G <output>235</output><input data-rgb="G" type="range" min="0" max="255" value="235"></label>
      <label class="rgb">B <output>250</output><input data-rgb="B" type="range" min="0" max="255" value="250"></label>
    </div>
  </div>
</div>
{{end}}`

const commandsPageHTML = `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Команды | CmdUI</title>
  <style>
    :root{font-family:"IBM Plex Sans","Segoe UI",sans-serif;color:#27312d;background:#f7f8f7;font-size:14px}
    *{box-sizing:border-box}body{margin:0}.layout{min-height:100vh;display:grid;grid-template-columns:220px minmax(0,1fr)}
    aside{background:#fff;border-right:1px solid #e1e6e2;padding:22px 14px;display:flex;flex-direction:column;gap:24px}
    .brand{padding:0 10px;font-weight:750;color:#16845b}.nav{display:grid;gap:4px}.nav a{padding:10px;border-radius:5px;color:#4b5750;text-decoration:none}
    .nav a.active,.nav a:hover{background:#eaf5ef;color:#126c4a}.account{margin-top:auto;border-top:1px solid #e7ebe8;padding:15px 9px 0;color:#667169}
    .account strong{display:block;color:#2d3831;margin-bottom:3px}.role{font-size:12px}.platform-label{display:block;margin-top:10px;padding-top:9px;border-top:1px solid #edf0ed;color:#66736b;font-size:11px;overflow-wrap:anywhere}.platform-label span{display:block;color:#879188;margin-bottom:2px}.logout{padding:0;border:0;background:none;color:#587064;font:inherit;text-decoration:underline;cursor:pointer;margin-top:12px}
    .workspace{min-width:0;padding:30px clamp(18px,4vw,54px);max-width:1500px;width:100%;margin:0 auto}.topline{display:flex;align-items:flex-start;justify-content:space-between;gap:20px;border-bottom:1px solid #e1e6e2;padding-bottom:20px;margin-bottom:24px}
    .eyebrow{color:#78837c;font-size:12px;text-transform:uppercase}.topline h1{font-size:26px;margin:5px 0 0;font-weight:650}.section-head{display:flex;align-items:center;justify-content:space-between;margin-bottom:13px}
    .section h2{font-size:17px;margin:0}.count,.muted{color:#78837c;font-size:12px}.notice{background:#eaf5ef;border:1px solid #cfe6d8;color:#246c4b;padding:11px 14px;margin-bottom:20px;border-radius:5px}
    .table-wrap{overflow-x:auto;background:#fff;border-block:1px solid #dfe5e0}table{width:100%;border-collapse:collapse;min-width:780px}th{text-align:left;color:#66736b;font-size:12px;padding:11px 12px;border-bottom:1px solid #e6ebe7}
    td{padding:12px;border-bottom:1px solid #edf0ed;vertical-align:middle}tr:last-child td{border:0}.cmd-name{font-weight:650}.description{font-size:12px;color:#77827b;margin-top:4px}
    .last-applied{font-size:11px;color:#78837c;margin-top:5px}.labels{display:flex;gap:5px;flex-wrap:wrap;margin-top:7px}.label-chip{display:inline-flex;padding:3px 7px;border-radius:4px;background:var(--label-color);font-size:11px;white-space:nowrap}
    .mono{font:12px ui-monospace,Consolas,monospace;overflow-wrap:anywhere;background:transparent;border-radius:0}.button{height:34px;padding:0 11px;border:1px solid #cbd6ce;border-radius:4px;background:#fff;color:#35433a;font:inherit;font-size:12px;cursor:pointer;text-decoration:none;display:inline-flex;align-items:center;justify-content:center}
    .button:hover{background:#f2f6f3}.button.primary{background:#147b54;color:#fff;border-color:#147b54}.button.danger{color:#a33b2c}.actions{display:flex;justify-content:flex-end;gap:6px;flex-wrap:wrap}.section{margin-bottom:32px}
    .last-run{background:#fff;border:1px solid #dce4de;padding:14px;margin-bottom:20px}.last-run-head{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}.last-run pre,.output{white-space:pre-wrap;overflow-wrap:anywhere;background:#f5f7f5;padding:10px;font:12px/1.5 ui-monospace,Consolas,monospace;max-height:220px;overflow:auto}.empty{padding:24px;text-align:center;color:#78837c}.status.running{background:#fff3d8;color:#76540a}.status.interrupted{background:#fce8d5;color:#825019}.interrupt-note{color:#825019;font-size:12px;margin:8px 0 0}
    .dialog-shell{width:min(780px,calc(100vw - 24px));max-height:88vh;padding:0;border:1px solid #d4ddd6;border-radius:8px;background:#f7f8f7;color:#27312d;box-shadow:0 18px 60px #1c30252b}
    .dialog-shell::backdrop{background:#18251f66}.dialog-head{position:sticky;top:0;background:#fff;border-bottom:1px solid #e1e6e2;padding:18px 22px;display:flex;align-items:center;justify-content:space-between;z-index:1}
    .dialog-head h2{margin:0;font-size:19px}.dialog-close{border:0;background:transparent;font-size:22px;color:#66736b;cursor:pointer}.dialog-body{padding:0 22px;overflow:auto}
    .command-form{display:block}.form-section{padding:16px 0;border-bottom:1px solid #e1e6e2}.form-section:last-of-type{border-bottom:0}.form-section-head{display:flex;justify-content:space-between;align-items:baseline;gap:12px;margin-bottom:12px}.form-section-head h3{font-size:14px;margin:0;font-weight:650}.form-section-head p{font-size:12px;color:#78837c;margin:0}.basic-grid{display:grid;grid-template-columns:minmax(0,1fr) 170px;gap:12px}.field label,.rgb label{display:block;font-size:12px;font-weight:600;margin:0 0 6px}
    .field input,.field select,.field textarea,.permission select{width:100%;border:1px solid #cfd8d1;background:#fff;border-radius:4px;padding:9px 10px;font:inherit;color:#27312d}
    .field textarea{min-height:70px;resize:vertical}.field .script-box{min-height:145px;font:12px/1.5 ui-monospace,Consolas,monospace}.full{grid-column:1/-1}
    .hint{font-size:12px;color:#78837c;margin:6px 0 0}.permission-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px 16px}.permission{min-width:0;padding:10px 0;border-bottom:1px solid #e8ede9}
    .permission>label{font-size:13px;font-weight:600}.permission input[type=checkbox]{accent-color:#147b54}.permission select{min-height:72px;margin-top:7px}
    .label-row{position:relative;border-top:1px solid #e7ebe8;padding:10px 0;display:grid;grid-template-columns:minmax(120px,1fr) minmax(120px,1fr) 36px 36px;gap:8px;align-items:end}
    .color-trigger{width:36px;height:36px;border:1px solid #cbd6ce;border-radius:4px;background:#fff;display:grid;place-items:center;cursor:pointer}.color-dot{width:19px;height:19px;border:1px solid #839088;border-radius:50%}
    .color-popover{position:absolute;z-index:5;right:42px;top:calc(100% - 2px);width:min(310px,calc(100vw - 70px));padding:12px;background:#fff;border:1px solid #d4ddd6;border-radius:6px;box-shadow:0 8px 24px #1c302520}.color-popover[hidden]{display:none}
    .popover-head{display:flex;justify-content:space-between;align-items:center;margin-bottom:10px;font-size:12px}.popover-close{border:0;background:none;color:#66736b;font-size:18px;cursor:pointer}.swatches{display:flex;gap:6px;flex-wrap:wrap}.swatch{width:23px;height:23px;border:1px solid #bfcac1;border-radius:50%;padding:0;cursor:pointer}.custom-color{display:flex;align-items:center;justify-content:space-between;margin-top:10px;font-size:12px}.color-picker{width:44px;height:30px;padding:2px;border:1px solid #cfd8d1;border-radius:4px;background:white}.rgb-controls{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:8px;margin-top:9px}.rgb{font-size:11px!important;color:#68756c}.rgb input{width:100%;padding:0;accent-color:#16845b}.rgb output{float:right;font-variant-numeric:tabular-nums}
    .form-actions{position:sticky;bottom:0;display:flex;justify-content:flex-end;gap:8px;align-items:center;flex-wrap:wrap;margin:0 -22px;padding:12px 22px;background:#fff;border-top:1px solid #e1e6e2}
	.command-toolbar{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:13px}.command-search{width:min(360px,100%);min-height:34px;padding:7px 10px;border:1px solid #cfd8d1;border-radius:4px;background:#fff;color:#27312d;font:inherit;font-size:12px}.command-layout{display:inline-flex;align-items:center;gap:4px}.command-layout-label{color:#78837c;font-size:12px}.command-layout button{min-width:30px;height:30px;padding:0 7px;border:1px solid #cbd6ce;border-radius:4px;background:#fff;color:#35433a;font:inherit;font-size:12px;cursor:pointer}.command-layout button[aria-pressed="true"]{border-color:#147b54;background:#eaf5ef;color:#126c4a}.command-board{overflow:visible;border:0;background:transparent}.command-board table{min-width:0}.command-board thead{display:none}.command-board tbody{display:grid;grid-template-columns:repeat(var(--command-columns,4),minmax(0,1fr));gap:12px}.command-board tr{display:flex;min-width:0;flex-direction:column;justify-content:space-between;min-height:170px;padding:14px;border:1px solid #dce4de;border-radius:6px;background:#fff;box-shadow:0 2px 8px #27364a0d}.command-board td{display:block;padding:0;border:0}.command-board td:not(:first-child):not(.command-actions-cell){display:none}.command-board .command-actions-cell{margin-top:14px}.command-board .description{white-space:pre-wrap}.command-board tr[hidden]{display:none}.command-board .empty{grid-column:1/-1;min-height:auto;text-align:center}
	@media(max-width:980px){.command-board tbody{grid-template-columns:repeat(min(var(--command-columns,4),3),minmax(0,1fr))}}
	@media(max-width:760px){.layout{grid-template-columns:1fr}aside{border-right:0;border-bottom:1px solid #e1e6e2;padding:13px 16px;gap:9px}.nav{grid-template-columns:repeat(3,minmax(0,1fr))}.account{display:flex;align-items:center;gap:10px;border:0;padding:0;margin:0;flex-wrap:wrap}.account strong{display:inline;margin:0}.logout{margin:0 0 0 auto}.workspace{padding:22px 16px}.permission-grid,.basic-grid{grid-template-columns:1fr}.dialog-head,.dialog-body{padding-left:15px;padding-right:15px}.form-actions{margin:0 -15px;padding:12px 15px}.topline h1{font-size:23px}.command-toolbar{align-items:stretch;flex-direction:column}.command-search{width:100%}.command-board tbody{grid-template-columns:1fr}}
  </style>
</head>
<body>
<div class="layout">
  <aside>
    <div class="brand">CMDUI <span style="color:#77827b;font-weight:400">/ Команды</span></div>
    <nav class="nav"><a class="active" href="/">Команды</a>{{if eq .User.Role "admin"}}<a href="/users">Пользователи</a>{{end}}{{if ne .User.Role "user"}}<a href="/runs">История запусков</a>{{end}}</nav>
    <div class="account"><strong>{{.User.Username}}</strong><span class="role">{{if eq .User.Role "admin"}}Администратор{{else if eq .User.Role "operator"}}Оператор{{else}}Пользователь{{end}}</span><div class="platform-label"><span>Платформа</span>{{.Platform}}</div><form method="post" action="/logout"><input type="hidden" name="csrf" value="{{.CSRF}}"><button class="logout" type="submit">Выйти</button></form></div>
  </aside>
  <main class="workspace">
    <header class="topline"><div><div class="eyebrow">Рабочая область</div><h1>Команды</h1></div>{{if eq .User.Role "admin"}}<button class="button primary" id="new-command" type="button">＋ Новая команда</button>{{end}}</header>
    {{with .Notice}}<div class="notice">{{.}}</div>{{end}}
    {{range .RunPanels}}<section class="last-run" data-run-status-url="/runs/{{.ID}}/status"><div class="last-run-head"><div><strong>Последний запуск: {{.CommandName}}</strong> <span class="status {{.Status}}" data-run-status>{{.Status}}</span>{{if eq .Status "running"}} <img class="run-spinner" data-run-spinner src="/static/icons/run-spinner.svg" alt="Выполняется">{{end}} <span class="muted">код <span data-run-exit-code>{{.ExitCode}}</span> · <span data-run-duration>{{.Duration}}</span></span></div>{{if and (eq $.User.Role "admin") (eq .Status "running")}}<form data-interrupt-form data-confirm-action data-confirm-variant="danger" data-confirm-title="Прервать запуск?" data-confirm-message="Текущий запуск будет остановлен." data-confirm-accept="Прервать" method="post" action="/runs/{{.ID}}/interrupt"><input type="hidden" name="csrf" value="{{$.CSRF}}"><button class="button danger" type="submit">Прервать</button></form>{{end}}</div><p class="interrupt-note" data-interrupt-note hidden>Системное прерывание отправлено; ожидается завершение процесса.</p><pre data-run-stdout {{if not .Stdout}}hidden{{end}}>{{.Stdout}}</pre><pre data-run-stderr {{if not .Stderr}}hidden{{end}}>{{.Stderr}}</pre></section>{{end}}
    <section class="section"><div class="section-head"><h2>Доступные команды</h2><span class="count">{{len .Commands}}</span></div>
      <div class="table-wrap"><table><thead><tr><th>Команда</th><th>Сценарий</th><th>Тайм-аут</th>{{if eq .User.Role "admin"}}<th>Видимость</th>{{end}}<th></th></tr></thead><tbody>
      {{range .Commands}}<tr><td><div class="cmd-name">{{iconGlyph .Icon}} {{.Name}}</div>{{if .Description}}<div class="description">{{linkify .Description}}</div>{{end}}{{if .LastAppliedBy}}<div class="last-applied">Последний запуск: {{.LastAppliedBy}}{{if hasTimestamp .LastAppliedAt}} · <time class="last-applied-time" data-local-timestamp datetime="{{.LastAppliedAt.Format "2006-01-02T15:04:05Z07:00"}}">{{.LastAppliedAt.Format "02.01.2006 15:04:05 UTC"}}</time>{{else}} · время не записано{{end}}</div>{{end}}<div class="labels">{{range .Labels}}<span class="label-chip" style="--label-color: {{.Color}}">{{.Key}}: {{.Value}}</span>{{end}}</div></td><td class="mono">{{if .Script}}Скрипт · {{len .Script}} символов{{else}}{{.Program}}{{range .Args}}<br>{{.}}{{end}}{{end}}</td><td>{{.TimeoutSeconds}} сек.</td>{{if eq $.User.Role "admin"}}<td><span class="muted">{{if .AccessAllUsers}}Все пользователи{{else if .AccessOperators}}Операторы{{else}}По списку{{end}}</span></td>{{end}}<td><div class="actions">{{if or (eq $.User.Role "admin") (and (eq $.User.Role "operator") .OperatorsCanRun)}}<form method="post" action="/commands/{{.ID}}/run"><input type="hidden" name="csrf" value="{{$.CSRF}}"><button class="button primary icon-action" type="submit" aria-label="{{if index $.RunningCommands .ID}}Команда уже выполняется{{else}}Запустить{{end}}" title="{{if index $.RunningCommands .ID}}Эта команда уже выполняется{{else}}Запустить{{end}}" {{if index $.RunningCommands .ID}}disabled{{end}}><img src="/static/icons/play.svg" alt="" aria-hidden="true"></button></form>{{end}}{{if eq $.User.Role "admin"}}<a class="button icon-action" href="/?edit={{.ID}}" aria-label="Редактировать" title="Редактировать"><img src="/static/icons/edit.svg" alt="" aria-hidden="true"></a><form data-confirm-action data-confirm-variant="danger" data-confirm-title="Удалить команду?" data-confirm-message="Команда будет удалена. История ее запусков останется." data-confirm-accept="Удалить" method="post" action="/admin/commands/{{.ID}}/delete"><input type="hidden" name="csrf" value="{{$.CSRF}}"><button class="button danger icon-action" type="submit" aria-label="Удалить" title="Удалить"><img src="/static/icons/trash.svg" alt="" aria-hidden="true"></button></form>{{end}}</div></td></tr>{{else}}<tr><td class="empty" colspan="5">Нет доступных для просмотра команд</td></tr>{{end}}
      </tbody></table></div>
    </section>
    {{if eq .User.Role "admin"}}<dialog class="dialog-shell" id="command-dialog" {{if .Editing}}open{{end}}>
      <header class="dialog-head"><div><h2>{{if .Editing}}Настроить команду{{else}}Новая команда{{end}}</h2><p class="hint">Основные данные, сценарий и права доступа</p></div><button class="dialog-close" data-close-dialog type="button" aria-label="Закрыть">×</button></header>
      <div class="dialog-body"><form method="post" action="/admin/commands" class="command-form" {{if .Editing}}data-confirm-script{{end}}><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="id" value="{{if .Editing}}{{.Editing.ID}}{{end}}">
        <section class="form-section"><div class="form-section-head"><h3>Основное</h3><p>Имя и идентификация команды</p></div><div class="basic-grid"><div class="field"><label for="name">Название</label><input id="name" name="name" required maxlength="100" value="{{if .Editing}}{{.Editing.Name}}{{end}}"></div><div class="field"><label for="timeout">Тайм-аут, секунд</label><input id="timeout" name="timeout_seconds" type="number" min="1" max="{{.MaxTimeoutSeconds}}" value="{{if .Editing}}{{.Editing.TimeoutSeconds}}{{else}}{{.DefaultTimeoutSeconds}}{{end}}" required></div><div class="field"><label for="description">Описание</label><input id="description" name="description" maxlength="300" value="{{if .Editing}}{{.Editing.Description}}{{end}}"></div><div class="field"><label for="command-icon">Значок</label><div class="icon-picker" data-icon-picker><input type="hidden" id="command-icon" name="icon" value="{{if .Editing}}{{.Editing.Icon}}{{end}}"><button class="button icon-picker-trigger" type="button" aria-haspopup="listbox" aria-expanded="false" aria-controls="command-icon-options"><span class="icon-picker-preview" data-icon-preview>{{if .Editing}}{{iconGlyph .Editing.Icon}}{{else}}<span class="icon-picker-empty">—</span>{{end}}</span><span data-icon-label>{{if .Editing}}{{iconName .Editing.Icon}}{{else}}Без значка{{end}}</span><span class="icon-picker-caret" aria-hidden="true">⌄</span></button><div class="icon-picker-options" id="command-icon-options" role="listbox" aria-label="Значки" hidden><button class="icon-option" type="button" role="option" data-icon-key="" data-icon-label="Без значка" aria-selected="false"><span class="icon-picker-glyph"><span class="icon-picker-empty">—</span></span><span>Без значка</span></button>{{range iconChoices}}<button class="icon-option" type="button" role="option" data-icon-key="{{.Key}}" data-icon-label="{{.Name}}" aria-selected="false"><span class="icon-picker-glyph">{{iconGlyph .Key}}</span><span>{{.Name}}</span></button>{{end}}</div></div></div></div></section>
        <section class="form-section"><div class="form-section-head"><h3>Скрипт</h3><p>Команды выполняются по порядку</p></div><div class="field"><textarea id="script" class="script-box" name="script" placeholder="Введите команды, по одной на строку">{{if .Editing}}{{.Editing.Script}}{{end}}</textarea><p class="hint">Скрипт запускается настроенным интерпретатором от имени процесса приложения.</p></div><details class="field" style="margin-top:10px"><summary>Один исполняемый файл</summary><div class="basic-grid" style="margin-top:10px"><div class="field"><label for="program">Разрешенный исполняемый файл</label><select id="program" name="program"><option value="">Выберите файл, если скрипт пустой</option>{{range .Executables}}<option value="{{.}}" {{if and $.Editing (eq $.Editing.Program .)}}selected{{end}}>{{.}}</option>{{end}}</select></div><div class="field"><label for="args">Аргументы, каждый с новой строки</label><textarea id="args" name="args">{{if .Editing}}{{range $i,$arg := .Editing.Args}}{{if $i}}{{"\n"}}{{end}}{{$arg}}{{end}}{{end}}</textarea></div></div></details></section>
        <section class="form-section"><div class="form-section-head"><h3>Labels</h3><button class="button" id="add-label" type="button" aria-label="Добавить label">＋ Добавить</button></div><div id="labels-editor">{{if and .Editing .Editing.Labels}}{{range .Editing.Labels}}{{template "label-row" .}}{{end}}{{end}}</div><template id="label-row-template">{{template "label-row" (blankLabel)}}</template><p class="hint">Добавьте пару ключ/значение; цвет настраивается рядом с меткой.</p></section>
        <section class="form-section"><div class="form-section-head"><h3>Доступ</h3><p>Просмотр и запуск задаются отдельно</p></div><div class="permission-grid"><div class="permission"><label><input type="checkbox" name="access_operators" {{if and .Editing .Editing.AccessOperators}}checked{{end}}> Доступна к просмотру операторам</label></div><div class="permission"><label><input type="checkbox" name="access_all_users" {{if and .Editing .Editing.AccessAllUsers}}checked{{end}}> Доступна к просмотру всем пользователям</label></div>
          <div class="permission"><label for="specific_users">Конкретным пользователям</label><select id="specific_users" name="specific_users" multiple>{{range .Users}}<option value="{{.Username}}" {{if and $.Editing (contains $.Editing.SpecificUsers .Username)}}selected{{end}}>{{.Username}} ({{.Role}})</option>{{end}}</select></div>
          <div class="permission"><label for="specific_operators">Конкретным операторам</label><select id="specific_operators" name="specific_operators" multiple>{{range .Operators}}<option value="{{.Username}}" {{if and $.Editing (contains $.Editing.SpecificOperators .Username)}}selected{{end}}>{{.Username}}</option>{{end}}</select></div>
          <div class="permission"><label><input type="checkbox" name="operators_can_run" {{if and .Editing .Editing.OperatorsCanRun}}checked{{end}}> Операторы могут запускать</label><p class="hint">Требуется также право просмотра.</p></div>
        </div></section>
        <footer class="form-actions"><button class="button" data-close-dialog type="button">Отмена</button><button class="button primary" type="submit">{{if .Editing}}Сохранить изменения{{else}}Создать команду{{end}}</button></footer>
      </form></div>
    </dialog><dialog class="confirm-dialog" data-confirm-dialog aria-labelledby="confirm-dialog-title" aria-describedby="confirm-dialog-message"><div class="confirm-dialog-content"><h2 id="confirm-dialog-title" data-confirm-title>Подтвердите действие</h2><p id="confirm-dialog-message" data-confirm-message></p><footer class="confirm-dialog-actions"><button class="button" type="button" data-confirm-cancel>Отмена</button><button class="button primary" type="button" data-confirm-accept>Подтвердить</button></footer></div></dialog>{{end}}
  </main>
</div><script src="/static/command-editor.js" defer></script><script src="/static/confirm-actions.js" defer></script></body></html>`

var CommandsTemplate = template.Must(template.New("commands").Funcs(template.FuncMap{
	"contains":     containsUsername,
	"blankLabel":   func() domain.Label { return domain.Label{Color: "#DCEBFA"} },
	"hasTimestamp": func(value time.Time) bool { return !value.IsZero() },
	"iconChoices":  func() []iconChoice { return popularIcons },
	"iconGlyph":    iconGlyph,
	"iconName":     iconName,
	"linkify":      linkifyDescription,
}).Parse(labelRowHTML + withSharedStylesheet(commandsPageHTML)))

var descriptionURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)
var descriptionMarkdownLinkPattern = regexp.MustCompile(`\[([^\]\r\n]+)\]\((https?://[^\s)]+)\)`)

func linkifyDescription(description string) template.HTML {
	var output strings.Builder
	position := 0
	for _, bounds := range descriptionMarkdownLinkPattern.FindAllStringSubmatchIndex(description, -1) {
		start, end := bounds[0], bounds[1]
		output.WriteString(renderDescriptionText(description[position:start]))
		label := description[bounds[2]:bounds[3]]
		urlText := description[bounds[4]:bounds[5]]
		output.WriteString(descriptionLink(urlText, label))
		position = end
	}
	output.WriteString(renderDescriptionText(description[position:]))
	return template.HTML(output.String())
}

func renderDescriptionText(text string) string {
	var output strings.Builder
	position := 0
	for _, bounds := range descriptionURLPattern.FindAllStringIndex(text, -1) {
		start, end := bounds[0], bounds[1]
		candidate := text[start:end]
		urlText := strings.TrimRight(candidate, ".,!?;:)]")
		if urlText == "" || descriptionLink(urlText, urlText) == html.EscapeString(urlText) {
			continue
		}
		output.WriteString(html.EscapeString(text[position:start]))
		output.WriteString(descriptionLink(urlText, urlText))
		output.WriteString(html.EscapeString(candidate[len(urlText):]))
		position = end
	}
	output.WriteString(html.EscapeString(text[position:]))
	return output.String()
}

func descriptionLink(urlText, label string) string {
	parsed, err := url.Parse(urlText)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return html.EscapeString(urlText)
	}
	return `<a href="` + html.EscapeString(urlText) + `" target="_blank" rel="noopener noreferrer">` + html.EscapeString(label) + `</a>`
}

type iconChoice struct {
	Key   string
	Name  string
	Glyph string
}

var popularIcons = []iconChoice{
	{Key: "go-color", Name: "Go · цветная"},
	{Key: "go-mono", Name: "Go · монохромная"},
	{Key: "grpcui-color", Name: "gRPC UI · цветная"},
	{Key: "grpcui-mono", Name: "gRPC UI · монохромная"},
	{Key: "podman-color", Name: "Podman · цветная"},
	{Key: "podman-mono", Name: "Podman · монохромная"},
	{Key: "terminal", Name: "Terminal", Glyph: "›_"},
	{Key: "database", Name: "База", Glyph: "▤"},
	{Key: "settings", Name: "Настройки", Glyph: "⚙"},
	{Key: "search", Name: "Поиск", Glyph: "⌕"},
	{Key: "check", Name: "Проверка", Glyph: "✓"},
	{Key: "play", Name: "Запуск", Glyph: "▶"},
	{Key: "refresh", Name: "Обновить", Glyph: "↻"},
	{Key: "star", Name: "Избранное", Glyph: "★"},
	{Key: "home", Name: "Главная", Glyph: "⌂"},
	{Key: "cloud", Name: "Облако", Glyph: "☁"},
	{Key: "chart", Name: "Метрики", Glyph: "▥"},
	{Key: "user", Name: "Учетная запись", Glyph: "♙"},
}

func iconName(key string) string {
	if key == "" {
		return "Без значка"
	}
	for _, choice := range popularIcons {
		if choice.Key == key {
			return choice.Name
		}
	}
	return key
}

var appIconAssets = map[string]template.HTML{
	"go-color":     `<img class="app-mark" src="/static/icons/go.svg" alt="" aria-hidden="true">`,
	"go-mono":      `<img class="app-mark icon-mono" src="/static/icons/go.svg" alt="" aria-hidden="true">`,
	"grpcui-color": `<img class="app-mark grpc-mark" src="/static/icons/grpc.svg" alt="" aria-hidden="true">`,
	"grpcui-mono":  `<img class="app-mark grpc-mark icon-mono" src="/static/icons/grpc.svg" alt="" aria-hidden="true">`,
	"podman-color": `<img class="app-mark podman-mark" src="/static/icons/podman.svg" alt="" aria-hidden="true">`,
	"podman-mono":  `<img class="app-mark podman-mark icon-mono" src="/static/icons/podman.svg" alt="" aria-hidden="true">`,
}

func iconGlyph(key string) template.HTML {
	if asset, ok := appIconAssets[key]; ok {
		return asset
	}
	if strings.HasPrefix(key, "custom-") {
		if id, err := strconv.ParseInt(strings.TrimPrefix(key, "custom-"), 10, 64); err == nil && id > 0 {
			return template.HTML(`<img class="app-mark custom-mark" src="/icons/custom/` + strconv.FormatInt(id, 10) + `" alt="" aria-hidden="true">`)
		}
	}
	for _, choice := range popularIcons {
		if choice.Key == key {
			return template.HTML(`<span class="legacy-icon">` + html.EscapeString(choice.Glyph) + `</span>`)
		}
	}
	return ""
}

func containsUsername(usernames []string, username string) bool {
	for _, candidate := range usernames {
		if candidate == username {
			return true
		}
	}
	return false
}
