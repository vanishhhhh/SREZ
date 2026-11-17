package mailer

import "html/template"

const recoveryText = "Мы получили запрос на восстановление доступа к аккаунту SREZ.\n\nЧтобы установить новый пароль, перейдите по ссылке.\n\nСсылка действительна 30 минут.\n\nЕсли вы не запрашивали восстановление, проигнорируйте это письмо.\n\n"

var recoveryHTML = template.Must(template.New("recovery").Parse(`<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="referrer" content="no-referrer"></head><body style="margin:0;background:#f5f4ef;color:#171715;font-family:Arial,sans-serif"><main style="max-width:560px;margin:40px auto;padding:40px;background:white;border:1px solid #ddd"><div style="font-size:42px;font-weight:900;letter-spacing:-3px">SREZ®</div><p style="font-size:11px;letter-spacing:2px;margin-top:32px">СВОЙ КРУГ. СВОЙ ВЫБОР.</p><h1 style="font-size:30px">Новый пароль.</h1><p>Мы получили запрос на восстановление доступа к аккаунту SREZ.</p><p>Чтобы установить новый пароль, перейдите по ссылке.</p><p style="margin:32px 0"><a href="{{.}}" style="display:inline-block;background:#171715;color:white;padding:18px 26px;text-decoration:none;font-weight:bold" rel="noreferrer">Восстановить пароль</a></p><p>Ссылка действительна 30 минут.</p><p style="color:#666">Если вы не запрашивали восстановление, проигнорируйте это письмо.</p></main></body></html>`))
