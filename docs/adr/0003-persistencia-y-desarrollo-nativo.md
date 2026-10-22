# ADR-0003 — Persistencia, migraciones y desarrollo nativo

- Estado: aceptado para etapa 2.
- Fecha: 2026-10-21.

## Contexto

El usuario solicita probar Go directamente con MySQL y Node locales, en la base existente `goconcept`. Se verificó MySQL 8.0.40 y base vacía. Las credenciales indicadas se guardan exclusivamente en `.env` ignorado, sin reproducirlas en documentación ni comandos de ejemplo. No se modifica el usuario MySQL existente.

## Decisiones

- `make dev` ejecuta Go nativo y carga el entorno local mediante un script pequeño sin dependencias. `make dev-compose` conserva la alternativa Compose obligatoria. Compose no se usa para validar esta etapa local. El binario sigue leyendo solo entorno, sin cargar archivos por sí mismo.
- Migración `000001_initial.up.sql` basada en DATA y rollback separado en orden inverso de dependencias. No hay migraciones automáticas al arrancar ni edición de migraciones aplicadas. MySQL DDL hace commits implícitos: un fallo parcial deja versión dirty y requiere inspección manual, nunca force automático.
- golang-migrate 4.20.1 se usa como biblioteca en `cmd/migrate`, con SQL embebido y credenciales desde configuración. Esto evita pasar un DSN secreto en argumentos del proceso. Solo el pool del migrador permite múltiples sentencias. `down` revierte una versión y requiere `ALLOW_MIGRATE_DOWN=1` por su pérdida de datos; los tests la ejercitan únicamente en bases temporales. Es una protección del comando, no una solicitud de aprobación para el desarrollo autorizado.
- sqlc 1.31.1 genera `database/sql` desde las migraciones y SQL explícito. Identidades y sesiones son globales por diseño; clientes, productos, membresías por tenant y auditoría comercial exigen tenant explícito en sus consultas/repositorios. Las escrituras de negocio usan tenant e ID. Listados iniciales ordenan por fecha/ID; las variantes de orden de los CRUD se implementarán en etapas 5/6.
- La validación del generador MySQL requiere placeholders posicionales en LIMIT/OFFSET y parámetros UTC para timestamps de consulta/baja; no se cambia la convención UTC. JSON nullable se genera como `[]byte` para soportar SQL NULL con database/sql. No se edita código generado.
- Se configura `clientFoundRows` para contar filas coincidentes y evitar falsos 404 en ediciones sin cambios (ADR-0001). Las bajas excluyen filas previamente borradas. Búsqueda literal con `LIKE ... ESCAPE '='`, parámetros y paginación limitada a 10/25/50.
- Las mutaciones de membresías del repositorio bloquean la fila tenant dentro de una transacción y preservan al menos un Owner activo. La autorización del actor y la política de transferencia de Owner siguen en etapa 4; estas funciones de persistencia no se exponen por HTTP.
- Seed de desarrollo transaccional: dos tenants, ocho usuarios (Owner/Admin/Viewer por empresa, usuario compartido y Platform Admin), ocho membresías y 100 clientes/productos por tenant. La opción de carga permite 10 000 por tenant. UUID v4 en Go. Las marcas de auditoría `seed.initialized` identifican el fixture; repetir el mismo seed no duplica ni resetea cuentas. Conflictos o fixtures parciales se rechazan sin sobrescribirlos.
- Contraseñas seed aleatorias, hash Argon2id con salt independiente y PHC versionado. Se añade `golang.org/x/crypto` para cumplir el algoritmo obligatorio: la biblioteca estándar no implementa Argon2id. Parámetros iniciales solo para seeds: 64 MiB, 3 iteraciones, 2 lanes, salt 16 bytes, clave 32 bytes; revisar/medir política de autenticación en D-02. Credenciales de desarrollo a archivo privado exclusivo `.local/seed-credentials.json`, nunca stdout/Git; fallo de escritura cancela seed. Repetir seed no regenera el archivo ni las contraseñas.
- Las pruebas crean y eliminan exclusivamente bases `gopanel_test_<aleatorio>` en MySQL local, sin rollback destructivo sobre la base de trabajo. CI podrá utilizar credenciales de prueba separadas con permisos de creación de estas bases. No parar ni reiniciar el MySQL local en smoke tests.

## Consecuencias

No se implementan login, sesiones HTTP, RBAC, CRUD HTTP ni dashboard en esta etapa. Se preparan sus contratos de persistencia. La política de campos y normalización SKU permanece en D-06; la unicidad actual sigue la collation de DATA. La invariante de Owner se protege en las operaciones de membresías disponibles; cualquier futura inactivación global de usuario debe protegerla también.

Fuentes: [migrate MySQL](https://github.com/golang-migrate/migrate/tree/v4.20.1/database/mysql), [sqlc MySQL](https://docs.sqlc.dev/en/v1.31.1/tutorials/getting-started-mysql.html), [Argon2id](https://pkg.go.dev/golang.org/x/crypto/argon2).
