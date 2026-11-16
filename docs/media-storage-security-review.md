# Security review — Media Phase 2 и исправления аудита

Дата: 2026-11-16. Область: итоговый рабочий diff ветки
`feat/media-storage-seaweedfs`, включая новые файлы, миграции 010–011, SQL triggers,
старые HTTP writer routes и S3 initialization. Review выполнялся чтением кода,
`git diff`, поиском call sites и сопоставлением с негативными тестами.
Это ручной review данного MVP, а не доказательство отсутствия всех уязвимостей.

**Результат:** подтверждённых незакрытых Critical/High в проверенном diff не найдено.

| Граница / риск | Проверка и результат |
| --- | --- |
| BOLA/IDOR | Avatar actor-only; product/color actual brand определяется SQL; target/input combinations ограничены; cross-user/brand/product/color — реальные негативные тесты |
| Revoke / parallel RBAC | Writе использует прежние Management.brand/withProduct/authorize; owner-only оформление, active owner/editor product; повторная проверка после S3, revoke во время PUT отклоняет commit |
| Mass assignment | Строгий JSON, user_id/key/bucket/status/public URL не принимаются; нет route чтения произвольного S3 key |
| Старые writer обходы | Старый complete — JWT + 410; нет вызова Presign из HTTP flow; profile/brand/product URL writers отклонены; unexpired staging PUT получает 403 от bucket policy |
| Size / framing | Early Content-Length >5 MiB — 413; missing/chunked — 411; MaxBytesReader + LimitReader независимо ограничивают прочитанные bytes; counter и реальный HEAD подтверждают отсутствие PUT/object у invalid sources |
| Content safety | Sniff + allowlist + DecodeConfig + полный Decode, 8192/12MP bounds, re-encode без source metadata, opaque JPEG/alpha PNG, ограниченный writer 8 MiB |
| CPU/RAM / slow clients | Один decoder и два отдельных delivery slots; backend request/response deadlines и server timeouts; slot выдачи удерживается до HTTP write; нет неограниченного body/read buffer |
| Duplicate / concurrent upload | Durable claim до PUT; конкурирующие реальные HTTP PUT создают одну S3-запись и одну ссылку; повтор complete-result не перепривязывает старый asset |
| SQL/S3 consistency | Journal до S3; атомарный ready+binding; реальный SQL trigger failure после PUT сохраняет старый avatar; ambiguous successful commit моделируется отдельно unit-тестом, без DELETE живого кандидата |
| Publication | Existing post-write aggregate checks; последнее фото published color и обязательный logo active brand защищены rollback; queue deletion также откатывается |
| GC locks / races | Writer parent→request→asset; GC блокирует только terminal/expired unreferenced assets, освобождает asset-lock до update request state и до S3 DELETE; late PUT обрабатывается конечным 24h resweep; выбранный failed DELETE остаётся обязательным |
| Visibility | Только ready + current reference; avatar self-only; brand/product public по текущему статусу; private/pending/retired unavailable, S3 anonymous GET — 403; no-store/nosniff |
| Secrets / logging | JSON не содержит bucket/key/credentials/presigned URL; SDK ошибки санитизированы, HTTP generic; entrypoint не печатает keys; автоматическая проверка SeaweedFS stdout/stderr до/после restart |
| Legacy / migrations | 001–010 не редактировались при исправлении аудита; 009→010 test сохраняет legacy HTTPS URL и controlled retention старых private ready; глобальная HTTPS validation прежняя |

Положительный SigV4 control сначала доказывает, что подписанный PUT работает
в разрешённом тестовом namespace; затем такой же signing flow для staging
получает 403 после RetireStaging. Это отличает запрет policy от случайной ошибки
подписи. Другие bucket-policy statements сохраняются; собственный Sid обновляется
идемпотентно. См. также
[SeaweedFS policy evaluation source](https://github.com/seaweedfs/seaweedfs/blob/4.54/weed/s3api/s3api_server.go).

Во время работы закрыты выявленные риски: unbounded direct upload, отсутствие
доменной ссылки у ready, URL-write обход, неоднозначность принадлежности, отсутствие
output cap, преждевременное освобождение buffer slot перед медленным HTTP write,
опасность инверсии request/asset locks в cleanup. Финальные проверки сведены в
[validation report](media-storage-validation.md).

## Остаточные ограничения / решения для production

* Нет распределённых rate limits, user/brand disk quotas и hard CPU sandbox.
  Конкретные bounds ограничивают данный процесс, но не заменяют budgets всего сервиса.
* Codec context не прерывает вычисления посреди Decode/Encode. Pixels/dimensions,
  slots и bounded input/output ограничивают объём работы; зависимости надо обновлять.
* Техническая ошибка после claim требует новой загрузки, если commit не был успешен.
  Потерянный ответ после успешного commit восстанавливается идемпотентным PUT retry.
* Тест неоднозначного successful commit — unit fault model, не разрыв реального
  PostgreSQL соединения в точке COMMIT. Реальный SQL rollback после S3 проверен.
* Нет внешнего browser/CDN/load test. HTTP сценарии используют Gin + httptest,
  PostgreSQL и SeaweedFS — реальные изолированные Docker сервисы.
* Backend управляет выделенным private bucket; его admin credentials находятся
  только на сервере. Production bootstrap/admin и runtime identities стоит разделить.
  Внешние административные изменения ACL/policy/objects находятся вне app RBAC.
* Audit/tombstone rows сохраняются; cleanup до 100 mandatory +10 resweep/мин должен наблюдаться
  и масштабироваться под нагрузку. Нужен независимый scheduled worker при scale-out.
* Не выполнены push, merge или миграция пользовательской БД. Пользовательские
  volumes не подключались к тестам и не удалялись.

## Повторный review пяти независимых находок

Первоначальная проверка Phase 2 пропустила AUD-M01/M02/M03/L01/L02. Предыдущий
результат не должен интерпретироваться как их отсутствие. В текущем проходе
проверяются именно исходные exploit probes и их новые регрессии; воспроизведение,
фиксы, результаты и оставшиеся допущения собраны в
[отчёте исправлений](media-storage-audit-fixes.md).

Privacy guard выполняется до SetMedia/Run: public Allow, public ACL и невозможность
прочитать ACL останавливают startup. Правила анализируются по разрешениям,
а не по валидности JSON; реальные anonymous GET/List controls подтверждают эффект.
Приватные external statements сохраняются. Условные публичные grants отклоняются
консервативно, даже когда могли бы оказаться безопасными на другом IAM evaluator.

Bucket mismatch проверяется при read/upload/cleanup (включая legacy 009), с явной
ошибкой конфигурации. GC не подтверждает удаление из другого bucket. Продолжение
работы после внешней административной смены policy/ACL не защищено постоянным
monitoring: credentials остаются доверенной границей MVP, нужен drift monitoring.

У сохраняемых старых URL, RBAC, visibility, ready/ref constraints и lock order
изменений в этой задаче нет. GC освобождает asset-lock перед request update и S3;
terminal resweep claim сохраняет durable retry, не удлиняя окно reconciliation.
CPU hard preemption, разделение admin/runtime identity, S3 startup coupling,
распределённые лимиты и CDN остаются следующими production-задачами.
