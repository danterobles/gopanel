# Guía de pruebas locales y compilación multiplataforma

Esta guía usa Go, Node y MySQL nativos, sin Docker. Ejecutar los comandos desde la raíz del repositorio. Los bloques `sh` son para macOS/Linux con shell POSIX; para el flujo completo en Windows, usar WSL2 con herramientas Linux. La ejecución de un binario Windows se describe por separado.

## 1. Preparar el entorno local

Requisitos del proyecto:

| Herramienta | Versión / uso |
|---|---|
| Go | 1.26.10, fijada en `go.mod` y el Makefile |
| Node.js | 22; validado con 22.23.2. npm instala desde el lockfile |
| MySQL | 8.0+; validado con 8.0.40 |
| Clientes MySQL | `mysql` y `mysqldump` 8 en `PATH`, necesarios para backup y E2E de recuperación |
| Make, shell y Python 3 | Comandos de desarrollo y scripts smoke |

Comprobar las herramientas:

```sh
GOTOOLCHAIN=go1.26.10 go version
node --version
npm --version
mysql --version
mysqldump --version
python3 --version
make --version
```

El Makefile selecciona el toolchain Go indicado; la primera ejecución puede descargarlo junto con módulos y generadores. Se necesita acceso a Internet para preparar esas dependencias.

Crear la configuración privada solo si todavía no existe:

```sh
test -f .env || cp .env.example .env
chmod 600 .env
```

Editar `.env` con los siguientes valores y completar `DB_PASSWORD` con la contraseña local, sin versionarla:

```dotenv
APP_ENV=development
HTTP_ADDR=127.0.0.1:8080
DB_HOST=127.0.0.1
DB_PORT=3306
DB_NAME=goconcept
DB_USER=root
DB_PASSWORD=
DB_TLS=false
```

El archivo se carga como asignaciones de shell confiables: entrecomillar valores con caracteres especiales. `root` corresponde al entorno local solicitado; para despliegue consultar el [runbook de operación](operacion.md).

La base `goconcept` ya está creada en el entorno actual. En una instalación nueva, crearla una sola vez con una cuenta autorizada; `-p` solicita la contraseña sin incluirla en el comando:

```sh
mysql -h 127.0.0.1 -u root -p -e 'CREATE DATABASE IF NOT EXISTS goconcept CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;'
```

## 2. Generar, migrar y arrancar

```sh
make generate
make migrate-up
make seed
make dev
```

`make generate` instala los paquetes npm fijados y genera assets, SQL Go y vistas Templ. `make migrate-up` aplica las migraciones pendientes; el servidor no las ejecuta automáticamente. `make seed` crea dos empresas, ocho cuentas y 100 clientes/productos por empresa. Repetirlo conserva un fixture completo existente y sus contraseñas; no cambia automáticamente su tamaño.

`make dev` compila `bin/gopanel`, carga `.env` y ejecuta el servidor en primer plano. Abrir [login local](http://localhost:8080/login). Ctrl+C detiene la aplicación y deja MySQL funcionando. Si el puerto está ocupado por otra ejecución de Go, detener esa ejecución o cambiar `HTTP_ADDR` en `.env`.

En otra terminal:

```sh
curl --fail http://127.0.0.1:8080/health/live
curl --fail http://127.0.0.1:8080/health/ready
```

Se espera HTTP 200. `live` comprueba el proceso y `ready` la conexión MySQL; `ready` no comprueba la versión del esquema, por lo que no sustituye las migraciones ni las pruebas funcionales.

## 3. Ingresar y probar los roles

Las contraseñas de aplicación están únicamente en `.local/seed-credentials.json`, en el campo `password` de cada correo. Son distintas de la contraseña de MySQL. No compartir ni incorporar este archivo a Git. Si falta después de un seed existente, volver a ejecutar el seed no recupera ni restablece las contraseñas.

| Cuenta | Qué probar |
|---|---|
| `owner.norte@gopanel.test` / `owner.sur@gopanel.test` | CRUD, miembros y configuración de su empresa |
| `admin.norte@gopanel.test` / `admin.sur@gopanel.test` | CRUD y dashboard; consulta de miembros |
| `viewer.norte@gopanel.test` / `viewer.sur@gopanel.test` | Solo consulta; sin mutaciones |
| `shared@gopanel.test` | Admin en Norte y Viewer en Sur; aislamiento y cambio de permisos |
| `platform@gopanel.test` | Administración global y `/platform/metrics`; sin membresías comerciales |

Recorrido mínimo:

1. Ingresar con `shared`, seleccionar Norte y crear un cliente y un producto.
2. Abrir el dashboard en otra pestaña y editar un recurso: las métricas deben refrescarse mediante SSE.
3. Cambiar a Sur: se muestra Viewer, desaparecen acciones de escritura y los datos de Norte no aparecen. Una URL de un recurso de Norte devuelve 404 desde Sur.
4. Revisar búsqueda, filtros, orden, paginación, validación visible y navegación con teclado.
5. Probar miembros con un Owner y métricas técnicas con Platform Admin. Cerrar sesión y comprobar que se solicita login al volver a una ruta protegida.

Los cambios manuales sí modifican `goconcept`. Las pestañas comparten sesión; para usuarios simultáneos usar perfiles de navegador separados. Más detalles en [cuentas y recorridos locales](pruebas-locales.md).

## 4. Ejecutar las verificaciones automáticas

Ejecutar en este orden, sin iniciar pruebas de carga en paralelo:

```sh
make lint test build
make test-integration
make smoke
make smoke-auth
make smoke-rbac
make test-e2e
```

| Verificación | Alcance y requisitos |
|---|---|
| `lint`, `test`, `build` | Formato, análisis estático, unitarias/HTTP y compilación nativa |
| `test-integration` | SQL, tenants, permisos, sesiones, CSRF y SSE con MySQL real |
| `smoke` | Health, assets y señales del proceso; no detiene MySQL |
| `smoke-auth`, `smoke-rbac` | Usan el seed privado y la base configurada; comprueban sesiones/permisos sin cambiar datos comerciales |
| `test-e2e` | Binario real con MySQL temporal, flujos HTTP y backup/restauración; no automatiza un navegador |

Integración y E2E crean/eliminan bases temporales `gopanel_test_*`. La cuenta debe poder crearlas y eliminarlas; se puede proporcionar `TEST_DB_USER` y `TEST_DB_PASSWORD` para una cuenta de pruebas independiente. No ejecutar rollback, restauración ni carga manual sobre `goconcept`.

Opcionales:

```sh
# Detector de carreras nativo: requiere CGO y compilador C disponibles.
GOTOOLCHAIN=go1.26.10 sh scripts/with-env.sh go test -race -tags=integration ./...

# Carga aislada: bases temporales, 50 usuarios, 10 000 registros/recurso/empresa.
make bench
```

El benchmark guarda su informe privado en `.local/benchmark-go.json`. Comparar resultados siguiendo el [protocolo](protocolo-benchmark.md); no equivalen a una garantía de rendimiento ni a una comparación ya realizada con Laravel.

## 5. Compilar para el equipo actual

```sh
make generate
make build
sh scripts/with-env.sh ./bin/gopanel
```

El servidor incorpora vistas y assets dentro del ejecutable. El migrador también incorpora los SQL. En destino no se necesitan Node, npm, Templ, sqlc ni Go para ejecutarlos; sí configuración y acceso a MySQL 8. El comando `backup` necesita además `mysqldump` instalado. En conexiones TLS se requiere un almacén de certificados de confianza adecuado en el sistema destino.

## 6. Compilar para otros sistemas y procesadores

`GOOS` selecciona el sistema operativo y `GOARCH` la arquitectura del procesador. `amd64` significa x86 de 64 bits, tanto Intel como AMD; `arm64` significa ARM de 64 bits, no ARM de 32 bits.

| Destino | `GOOS` | `GOARCH` | Carpeta de salida |
|---|---|---|---|
| macOS Apple Silicon | darwin | arm64 | `bin/darwin-arm64` |
| macOS Intel | darwin | amd64 | `bin/darwin-amd64` |
| Linux Intel/AMD 64 bits | linux | amd64 | `bin/linux-amd64` |
| Linux ARM64 | linux | arm64 | `bin/linux-arm64` |
| Windows Intel/AMD 64 bits | windows | amd64 | `bin/windows-amd64` |
| Windows ARM64 | windows | arm64 | `bin/windows-arm64` |

Primero ejecutar `make generate` para la máquina que compila. No exportar `GOOS`/`GOARCH` de otro destino durante la generación: los generadores deben ejecutarse en el sistema anfitrión.

Ejemplo para compilar solo el servidor Linux x86-64 desde macOS o Linux:

```sh
mkdir -p bin/linux-amd64
GOTOOLCHAIN=go1.26.10 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -o bin/linux-amd64/gopanel ./cmd/server
```

Para generar los cuatro ejecutables en los seis destinos de la tabla, ejecutar este bloque en una shell POSIX. Se usa un subshell para no dejar variables de destino en la terminal:

```sh
make generate
(
  set -eu
  export GOTOOLCHAIN=go1.26.10 CGO_ENABLED=0
  for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
    export GOOS="${target%/*}" GOARCH="${target#*/}"
    destination="bin/$GOOS-$GOARCH"
    extension=""
    if [ "$GOOS" = windows ]; then extension=".exe"; fi
    mkdir -p "$destination"
    for command in server migrate seed backup; do
      name="$command"
      if [ "$command" = server ]; then name=gopanel; fi
      go build -trimpath -o "$destination/$name$extension" "./cmd/$command"
    done
  done
)
```

Cada carpeta contiene `gopanel`, `migrate`, `seed` y `backup`, con extensión `.exe` en Windows. Los resultados quedan en `bin/`, excluido de Git. `CGO_ENABLED=0` permite compilar estos comandos sin una cadena C del destino; no empaqueta MySQL ni los clientes externos de backup.

Inspeccionar un resultado y consultar las combinaciones que reconoce el toolchain:

```sh
file bin/linux-amd64/gopanel
GOTOOLCHAIN=go1.26.10 go version -m bin/linux-amd64/gopanel
GOTOOLCHAIN=go1.26.10 go tool dist list
```

La lista del toolchain no implica soporte probado del proyecto para todos esos destinos. No ejecutar un binario Linux/Windows directamente en macOS; copiarlo a un equipo o VM con el sistema y arquitectura correspondientes. Las pruebas `go test` normales se ejecutan para el anfitrión, no validan por sí solas un binario de otro sistema.

## 7. Ejecutar el resultado en el destino

Para macOS/Linux, copiar los ejecutables del destino, preparar MySQL y configurar las variables. Si se dispone del repositorio y su `.env` privado, desde su raíz, por ejemplo en Linux ARM64:

```sh
chmod +x bin/linux-arm64/gopanel bin/linux-arm64/migrate
sh scripts/with-env.sh ./bin/linux-arm64/migrate up
sh scripts/with-env.sh ./bin/linux-arm64/gopanel
```

Sin repositorio, proporcionar las variables mediante el entorno del proceso o el gestor de servicios. El ejecutable por sí mismo **no lee `.env`**. En una base nueva de desarrollo, ejecutar también el `seed` del destino con `APP_ENV=development`; el archivo de credenciales se crea relativo al directorio actual. No sembrar datos de prueba en producción.

En Windows, ejemplo PowerShell desde la raíz del repositorio para x86-64:

```powershell
$env:APP_ENV = "development"
$env:HTTP_ADDR = "127.0.0.1:8080"
$env:DB_HOST = "127.0.0.1"
$env:DB_PORT = "3306"
$env:DB_NAME = "goconcept"
$env:DB_USER = "root"
$env:DB_TLS = "false"
$credential = Get-Credential -UserName "root" -Message "Credenciales de MySQL local"
$env:DB_PASSWORD = $credential.GetNetworkCredential().Password
.\bin\windows-amd64\migrate.exe up
if ($LASTEXITCODE -ne 0) { throw "Falló la migración" }
.\bin\windows-amd64\gopanel.exe
```

Para Windows ARM64 cambiar la carpeta a `windows-arm64`. No cargar el archivo shell `.env` como script PowerShell. En WSL2 corresponde usar el binario Linux y una dirección MySQL alcanzable desde WSL; `127.0.0.1` debe referirse al servidor accesible en ese entorno.

En cada destino, comprobar health, login, CRUD, aislamiento, SSE y reinicio con persistencia antes de considerar validada su ejecución. Mantener una sola instancia Go por el broker SSE en memoria. La instalación como servicio y la configuración productiva se tratan en el [runbook](operacion.md).

## 8. Alcance de la comprobación de esta guía

El 22 de octubre de 2026 se comprobó la compilación con Go 1.26.10 y `CGO_ENABLED=0` de los cuatro comandos para los seis destinos de la tabla: 24 ejecutables. La máquina anfitriona es macOS ARM64. Las pruebas funcionales locales previas corresponden al entorno nativo descrito en la [validación de etapa 8](validacion-etapa-8.md).

Compilar para Linux, Windows o macOS Intel no demuestra su funcionamiento en esos equipos. Quedan pendientes las pruebas de ejecución por destino; los scripts POSIX, las señales y herramientas del benchmark también deben validarse allí. No se ha validado instalación como servicio Windows ni distribución firmada de macOS.
