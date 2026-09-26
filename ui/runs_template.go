package ui

import "html/template"

const runsPageHTML = `<!doctype html>
<html lang="ru">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>История запусков | CmdUI</title>
<style>
:root{font-family:"IBM Plex Sans","Segoe UI",sans-serif;color:#27312d;background:#f7f8f7;font-size:14px}*{box-sizing:border-box}body{margin:0}.layout{min-height:100vh;display:grid;grid-template-columns:220px minmax(0,1fr)}aside{background:#fff;border-right:1px solid #e1e6e2;padding:22px 14px;display:flex;flex-direction:column;gap:24px}.brand{padding:0 10px;font-weight:750;color:#16845b}.nav{display:grid;gap:4px}.nav a{padding:10px;border-radius:5px;color:#4b5750;text-decoration:none}.nav a.active,.nav a:hover{background:#eaf5ef;color:#126c4a}.account{margin-top:auto;border-top:1px solid #e7ebe8;padding:15px 9px 0;color:#667169}.account strong{display:block;color:#2d3831;margin-bottom:3px}.role{font-size:12px}.platform-label{display:block;margin-top:10px;padding-top:9px;border-top:1px solid #edf0ed;color:#66736b;font-size:11px;overflow-wrap:anywhere}.platform-label span{display:block;color:#879188;margin-bottom:2px}.logout{padding:0;border:0;background:none;color:#587064;font:inherit;text-decoration:underline;cursor:pointer;margin-top:12px}.workspace{min-width:0;padding:30px clamp(18px,4vw,54px);max-width:1500px;width:100%;margin:0 auto}.topline{border-bottom:1px solid #e1e6e2;padding-bottom:20px;margin-bottom:24px}.eyebrow{color:#78837c;font-size:12px;text-transform:uppercase}.topline h1{font-size:26px;margin:5px 0 0;font-weight:650}.filters{display:flex;gap:8px;flex-wrap:wrap;margin-bottom:14px}.filters input,.filters select{height:36px;min-width:130px;border:1px solid #cfd8d1;background:#fff;border-radius:4px;padding:0 9px;font:inherit;color:#27312d}.button{height:36px;padding:0 12px;border:1px solid #cbd6ce;border-radius:4px;background:#fff;color:#35433a;font:inherit;font-size:12px;cursor:pointer;text-decoration:none}.table-wrap{overflow:auto;background:#fff;border-block:1px solid #dfe5e0}table{width:100%;border-collapse:collapse;min-width:760px}th{text-align:left;color:#66736b;font-size:12px;font-weight:600;padding:11px 12px;border-bottom:1px solid #e6ebe7;white-space:nowrap}td{padding:12px;border-bottom:1px solid #edf0ed;vertical-align:top}tr:last-child td{border:0}.status{display:inline-block;padding:3px 7px;border-radius:3px;background:#edf1ee;color:#59645d;font-size:12px}.status.success{background:#e6f4eb;color:#1c714b}.status.failed,.status.timeout{background:#fff0ee;color:#a33b2c}details summary{cursor:pointer;color:#536158;font-size:12px}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#f6f8f6;padding:10px;max-height:220px;overflow:auto;font:12px/1.5 ui-monospace,Consolas,monospace}.empty{text-align:center;color:#78837c;padding:24px}@media(max-width:760px){.layout{grid-template-columns:1fr}aside{border-right:0;border-bottom:1px solid #e1e6e2;padding:13px 16px;gap:9px}.nav{grid-template-columns:repeat(3,minmax(0,1fr))}.account{display:flex;align-items:center;gap:10px;border:0;padding:0;margin:0;flex-wrap:wrap}.account strong{display:inline;margin:0}.logout{margin:0 0 0 auto}.workspace{padding:22px 16px}.topline h1{font-size:23px}}
</style>
<style>
.history-filter-panel{display:grid;grid-template-columns:repeat(5,minmax(0,1fr)) auto;align-items:end;gap:12px;margin:0 0 18px;padding:16px;background:#fff;border:1px solid #e1e6eb;border-radius:8px}
.filter-field{display:grid;gap:6px;min-width:0;color:#657287;font-size:11px;font-weight:650}
.filter-field input,.filter-field select{box-sizing:border-box;width:100%;min-width:0;height:40px;padding:0 10px;border:1px solid #cbd3dc;border-radius:6px;background:#fff;color:#4c5773;font:inherit;box-shadow:none}
.filter-field input::placeholder{color:#9aa5b3}
.filter-actions{display:flex;align-items:center;gap:8px}
.filter-actions .button{height:40px;border-radius:6px}
.history-results-toolbar{display:flex;align-items:center;justify-content:space-between;gap:16px;margin:0 2px 12px;color:#657287;font-size:13px}
.history-results-summary{font-variant-numeric:tabular-nums}
.history-results-summary strong{color:#243a52;font-weight:650}
.page-size-control{display:flex;align-items:center;gap:10px;margin:0}
.page-size-control label{color:#657287;font-size:12px;white-space:nowrap}
.page-size-control select{height:36px;min-width:78px;padding:0 26px 0 10px;border:1px solid #cbd3dc;border-radius:6px;background:#fff;color:#4c5773;font:inherit}
.page-size-control .button{height:36px;border-radius:6px}
.table-wrap{border-radius:6px 6px 0 0}
.pagination{margin-top:0;padding:12px 16px;background:#fff;border:1px solid #e1e6eb;border-top:0;border-radius:0 0 6px 6px;color:#657287;font-size:13px}
.pagination-summary{font-variant-numeric:tabular-nums}
.pagination-controls{gap:10px}
.pagination-current{min-width:130px;height:36px;display:inline-flex;align-items:center;justify-content:center;padding:0 12px;border-radius:6px;background:#f3f6f8;font-variant-numeric:tabular-nums}
.pagination .button{min-width:86px;height:36px;border-radius:6px}
@media(max-width:1050px){.history-filter-panel{grid-template-columns:repeat(3,minmax(0,1fr))}.filter-actions{grid-column:1/-1;justify-content:flex-end}}
@media(max-width:650px){.history-filter-panel{grid-template-columns:repeat(2,minmax(0,1fr));gap:10px;padding:12px}.filter-actions{grid-column:1/-1}.history-results-toolbar{align-items:flex-start;flex-direction:column}.page-size-control{width:100%;justify-content:space-between}.pagination{align-items:stretch}.pagination-controls{justify-content:space-between;gap:6px}.pagination-current{min-width:0;flex:1}.pagination .button{min-width:70px;padding-inline:8px}}
@media(max-width:390px){.history-filter-panel{grid-template-columns:1fr}.filter-actions{flex-wrap:wrap}.filter-actions .button{flex:1}}
</style>
</head>
<body><div class="layout">
<aside><div class="brand">CMDUI <span style="color:#77827b;font-weight:400">/ История</span></div><nav class="nav"><a href="/">Команды</a>{{if eq .User.Role "admin"}}<a href="/users">Пользователи</a>{{end}}<a class="active" href="/runs">История запусков</a></nav><div class="account"><strong>{{.User.Username}}</strong><span class="role">{{if eq .User.Role "admin"}}Администратор{{else}}Оператор{{end}}</span><div class="platform-label"><span>Платформа</span>{{.Platform}}</div><form method="post" action="/logout"><input type="hidden" name="csrf" value="{{.CSRF}}"><button class="logout" type="submit">Выйти</button></form></div></aside>
<main class="workspace"><header class="topline"><div class="eyebrow">Мониторинг</div><h1>История запусков</h1></header>
{{if eq .User.Role "admin"}}<form class="history-filter-panel" method="get" action="/runs"><label class="filter-field"><span>Пользователь</span><input name="user" list="run-user-suggestions" aria-label="Пользователь" placeholder="Все пользователи" value="{{.Filter.Username}}"></label><datalist id="run-user-suggestions">{{range .Usernames}}<option value="{{.}}">{{end}}</datalist><label class="filter-field"><span>Команда</span><input name="command" aria-label="Команда" placeholder="Название команды" value="{{.Filter.Command}}"></label><label class="filter-field"><span>Статус</span><select name="status"><option value="">Все статусы</option><option value="running" {{if eq .Filter.Status "running"}}selected{{end}}>Выполняется</option><option value="success" {{if eq .Filter.Status "success"}}selected{{end}}>Успех</option><option value="failed" {{if eq .Filter.Status "failed"}}selected{{end}}>Ошибка</option><option value="timeout" {{if eq .Filter.Status "timeout"}}selected{{end}}>Тайм-аут</option><option value="interrupted" {{if eq .Filter.Status "interrupted"}}selected{{end}}>Прервано</option></select></label><label class="filter-field"><span>Начало с</span><input type="date" name="from" aria-label="С даты" value="{{.Filter.From}}"></label><label class="filter-field"><span>Начало по</span><input type="date" name="to" aria-label="По дату" value="{{.Filter.To}}"></label><input type="hidden" name="page_size" value="{{.PageSize}}"><div class="filter-actions"><button class="button primary" type="submit">Показать</button><a class="button" href="/runs?page_size={{.PageSize}}">Сбросить</a></div></form>{{end}}
<div class="history-results-toolbar"><div class="history-results-summary">{{if gt .TotalRuns 0}}Показано <strong>{{.StartRun}}–{{.EndRun}}</strong> из <strong>{{.TotalRuns}}</strong>{{else}}Запусков пока нет{{end}}</div><form class="page-size-control" method="get" action="/runs">{{if eq .User.Role "admin"}}<input type="hidden" name="user" value="{{.Filter.Username}}"><input type="hidden" name="command" value="{{.Filter.Command}}"><input type="hidden" name="status" value="{{.Filter.Status}}"><input type="hidden" name="from" value="{{.Filter.From}}"><input type="hidden" name="to" value="{{.Filter.To}}">{{end}}<label for="history-page-size">Записей на странице</label><select id="history-page-size" name="page_size" aria-label="Записей на странице">{{range $size := .PageSizes}}<option value="{{$size}}" {{if eq $.PageSize $size}}selected{{end}}>{{$size}}</option>{{end}}</select><button class="button" type="submit">Применить</button></form></div>
<div class="table-wrap"><table><thead><tr>{{if eq .User.Role "admin"}}<th>Пользователь</th>{{end}}<th>Команда</th><th>Статус</th><th>Код</th><th>Начало</th><th>Длительность</th>{{if eq .User.Role "admin"}}<th>Запуск</th>{{end}}<th>Вывод</th></tr></thead><tbody>{{range .Runs}}<tr>{{if eq $.User.Role "admin"}}<td>{{.Username}}</td>{{end}}<td>{{.CommandName}}</td><td><span class="status {{.Status}}">{{statusLabel .Status}}</span></td><td>{{.ExitCode}}</td><td>{{.StartedAt.Format "2006-01-02 15:04:05"}}</td><td>{{.Duration}}</td>{{if eq $.User.Role "admin"}}<td><details><summary>Показать запуск</summary>{{if .CommandScript}}<pre>{{.CommandScript}}</pre>{{else if .Program}}<pre>{{.Program}}{{range .Args}}{{"\n"}}{{.}}{{end}}</pre>{{else}}<span class="muted">Параметры не сохранены</span>{{end}}</details></td>{{end}}<td><details><summary>Показать вывод</summary>{{if .Stdout}}<pre>{{.Stdout}}</pre>{{end}}{{if .Stderr}}<pre>{{.Stderr}}</pre>{{end}}</details></td></tr>{{else}}<tr><td class="empty" colspan="{{if eq .User.Role "admin"}}8{{else}}6{{end}}">Запусков пока нет</td></tr>{{end}}</tbody></table></div>
{{if gt .TotalRuns 0}}<nav class="pagination" aria-label="Страницы истории"><div class="pagination-controls">{{if .PreviousURL}}<a class="button" href="{{.PreviousURL}}">← Назад</a>{{else}}<button class="button" type="button" disabled>← Назад</button>{{end}}<span class="pagination-current">Страница <strong>{{.Page}}</strong> из {{.PageCount}}</span>{{if .NextURL}}<a class="button" href="{{.NextURL}}">Далее →</a>{{else}}<button class="button" type="button" disabled>Далее →</button>{{end}}</div></nav>{{end}}
</main></div></body></html>`

var RunsTemplate = template.Must(template.New("runs").Funcs(template.FuncMap{"statusLabel": statusLabel}).Parse(withSharedStylesheet(runsPageHTML)))

func statusLabel(status string) string {
	switch status {
	case "running":
		return "Выполняется"
	case "success":
		return "Успешно"
	case "failed":
		return "Ошибка"
	case "timeout":
		return "Тайм-аут"
	case "interrupted":
		return "Прервано"
	default:
		return status
	}
}
