# Cuentas y pruebas locales

Abrir http://localhost:8080/login. Cuentas verificadas activas en goconcept al iniciar etapa 8. Las contraseñas de aplicación están en el archivo privado `.local/seed-credentials.json` (campo password de cada email). No son la contraseña de MySQL. No se cambian al ejecutar nuevamente make seed y nunca deben copiarse al repositorio.

| Usuario | Empresa norte | Empresa sur | Uso |
|---|---|---|---|
| owner.norte@gopanel.test | Owner | Sin membresía | CRUD, miembros y configuración de Norte |
| owner.sur@gopanel.test | Sin membresía | Owner | CRUD, miembros y configuración de Sur |
| admin.norte@gopanel.test | Admin | Sin membresía | CRUD y consulta de miembros de Norte |
| admin.sur@gopanel.test | Sin membresía | Admin | CRUD y consulta de miembros de Sur |
| viewer.norte@gopanel.test | Viewer | Sin membresía | Solo consulta en Norte |
| viewer.sur@gopanel.test | Sin membresía | Viewer | Solo consulta en Sur |
| shared@gopanel.test | Admin | Viewer | Comprobar cambio de permisos y aislamiento al cambiar empresa |
| platform@gopanel.test | Sin membresía | Sin membresía | Administración global en /platform y métricas en /platform/metrics |

Owner puede crear/editar/dar de baja clientes y productos, consultar dashboard, gestionar miembros existentes (Admin/Viewer) y cambiar el nombre de su empresa. No puede transferir la propiedad desde la UI ni eliminar el último Owner activo.

Admin tiene CRUD y dashboard y puede consultar miembros; no cambia roles, no agrega miembros ni modifica la configuración de empresa. Viewer consulta dashboard, clientes/productos y sus filtros; no crea, edita ni da de baja. Platform Admin crea/administra empresas y observa telemetría; su rol global no concede acceso comercial a Norte o Sur.

## Recorridos recomendados

1. Ingresar con shared, seleccionar Norte y crear un cliente/producto. Dejar el dashboard abierto en otra pestaña: debe actualizar sus tarjetas por SSE.
2. Cambiar a Sur con shared. Debe mostrarse Viewer y desaparecer acciones de escritura. Los recursos creados en Norte no aparecen en Sur; una URL de otro tenant devuelve 404.
3. Ingresar con owner.norte para consultar miembros/configuración. Cambiar un rol o estado revoca las sesiones del usuario afectado; debe iniciar sesión nuevamente. La gestión no crea cuentas ni envía invitaciones.
4. Ingresar con platform: desde el selector abrir Administración de plataforma. Puede observar /platform/metrics, pero /app pide seleccionar una empresa de la que sea miembro.
5. Usar teclado: Tab para controles, Enter para acciones/enlaces y la tabla diaria del dashboard. Revisar errores visibles, cancelación/confirmación y estados vacíos.

Dos pestañas del mismo navegador comparten cookie: cambiar empresa o cerrar sesión en una afecta la sesión común. Para usuarios simultáneos usar perfiles/contextos de navegador separados. Una sesión inactiva expira aunque el dashboard reciba eventos de fondo. Los cambios comerciales que hagas manualmente en goconcept son reales; no ejecutar herramientas de carga o rollback sobre ella.

## Verificación repetible

- `make test`: unitarias.
- `make test-integration`: HTTP/SQL/RBAC/CSRF/SSE con bases temporales.
- `make test-e2e`: binario real, MySQL temporal, recorrido completo y ensayo de backup/restauración. No es un runner de navegador.
- `make bench`: 50 identidades, 2 empresas, 10 000 registros de cada recurso por empresa, 25 filas HTML. Crea/elimina sus bases de prueba; informe en `.local/benchmark-go.json`. Ejecutarlo sin otras pruebas/carga en paralelo.
- `make backup`: snapshot privado de la base configurada en `.env`, archivo nuevo en `.local/backups/`; no sobrescribe ni restaura datos.

La suite automatizada E2E HTTP y la revisión de navegador se complementan. No se afirma una auditoría WCAG completa ni una comparación Laravel/Filament sin contraparte.
