# Validación de la etapa 3

Fecha: 2026-10-21. Entorno: macOS ARM64, Go 1.26.10, Node 22.23.2 y MySQL local 8.0.40. Pruebas ejecutadas sin Docker.

## Resultado y contratos

Login/logout y selector de empresa funcionales con Templ, formularios HTML normales y HTMX local. `/app` muestra solo la empresa y el rol verificados; aún no es el dashboard. Sesiones persistidas en MySQL con hash de token, expiración absoluta/inactividad, touch condicionado, rotación al seleccionar tenant y revocación al salir. Se preservaron los usuarios y hashes seed existentes.

ADR-0004 cierra D-02 con política configurable y D-03 difiriendo recuperación automática, sin correo ni servicio externo. No hay nuevas dependencias. La migración 000002 agrega `login_rate_limits`; 000001 no se modificó. Up aplicada a `goconcept`, versión 2 limpia. Down 2 elimina únicamente contadores de login; rollback se ensayó en bases desechables.

## Evidencia

| Caso / comando | Resultado |
|---|---|
| `make generate`, gofmt, `make lint test build`, vet con tag integration | Correctos; Templ, Tailwind y sqlc fijados. |
| `make test-integration` | Aprobado contra MySQL real en bases `gopanel_test_*`, eliminadas al terminar. |
| `sh scripts/with-env.sh go test -race -tags=integration ./... -count=1` | Aprobado, incluidas admisión concurrente, seguridad HTTP y regresión de persistencia. |
| Login normal/HTMX | Credenciales correctas crean sesión/cookie; desconocidas/inactivas devuelven 401 genérico; formato inválido 422; fragmentos sin documento completo ni contraseña. |
| CSRF/origen | Login sin token, token en query, origen ajeno, logout sin token y CSRF obsoleto rechazados; HX-Request no evita autenticación. Body excesivo devuelve 413. |
| Cookies | HttpOnly, SameSite=Lax, Path=/; bajo TLS de prueba, Secure y prefijo __Host- sin Domain. Token opaco no aparece en HTML. |
| Selector/tenant | Usuario compartido ve solo empresas activas propias; IDs/roles/usuarios manipulados no escalan privilegios; empresa ajena devuelve 403 sin filtrar datos; membresía revocada pierde acceso en siguiente GET. Platform Admin sin membresías ve estado vacío. |
| Rotación y logout | Login repetido revoca token anterior; cambiar tenant rota token/CSRF y preserva creación/vencimiento absoluto; logout elimina fila; GET logout no muta y POST requiere CSRF. |
| Caducidad/touch | Reloj controlado verifica límites exactos absoluto/idle; no escribe actividad en cada request y sí al vencer intervalo; cookie manipulada/usuario inactivo no autentican. |
| Rate limit | Seis intentos concurrentes con límite 2: dos procesados y cuatro rechazados; reset en frontera de minuto y limpieza de buckets; HTTP 429/Retry-After incluso variando X-Forwarded-For. |
| Auditoría/errores | Login/logout/denegaciones sin contraseñas, correo ni tokens; éxito asociado al actor correcto. Fallo de auditoría impide commit de login y no deja sesión. |
| `make smoke` | Binario nativo, assets/health y SIGTERM correctos; conexión DB inalcanzable simulada sin detener MySQL. |
| `make smoke-auth` | Usuario seed compartido: login, Norte/Admin, reinicio real del proceso Go con la misma cookie, sesión/CSRF recuperados de MySQL, Sur/Viewer, CSRF anterior rechazado y logout. Sin mostrar credenciales. |
| Navegador real | Login enviado por teclado (Enter); HTMX mostró error genérico y limpió contraseña conservando foco. Etiquetas/campos visibles y foco contrastado. Capturas privadas en `.local/auth-login.jpg` y `.local/auth-login-htmx.jpg`. |
| Argon2id local | `go test ./internal/platform/auth/password -run '^$' -bench BenchmarkVerify -benchtime=5x -benchmem`: Apple M2 Max, 62.9554 ms/op, 67 113 804 B/op y 52 allocs/op. Perfil 64 MiB/3/2; no es benchmark de carga ni garantiza latencia de producción. |

## Límites y siguiente etapa

No se ejecutó CI remoto ni se reconstruyó Docker para esta entrega nativa. La verificación visual de teclado no certifica por sí sola WCAG 2.2 AA; la auditoría completa sigue en etapa 8. No se implementaron membresías HTTP, RBAC por acción/recurso, CRUD, dashboard o SSE.

Las elevaciones de roles futuras deben gestionar revocación/rotación en el servicio de etapa 4. SSE deberá revalidar conexiones largas según D-08; los límites de escritura/proxy siguen pendientes de esa etapa. Readiness comprueba conectividad MySQL, no versión de esquema.

Siguiente paso: etapa 4, cerrar D-04/D-05 y aplicar políticas por acción/recurso y gestión segura de miembros sobre el ActorContext ya verificado.
