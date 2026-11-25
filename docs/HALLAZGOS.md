# Hallazgos de validación y cómo corregirlos

- **Proyecto**: Soul App (`github.com/jesusuzcategui/soul-app`) · Go + Fyne
- **Commit auditado**: `8545eda` · versión embebida `0.1.2`
- **Fecha de la auditoría**: 2026-11-25
- **Alcance**: código Go (~4.100 LOC), tests, CI (`.github/workflows/release.yml`),
  documentación (`README.md`, `SPEC.md`, `PLAN.md`), dependencias y ejecución real
  en Linux/Wayland.
- **Para qué sirve este documento**: que otra persona o agente pueda aplicar las
  correcciones sin repetir la auditoría. Cada hallazgo incluye evidencia,
  corrección propuesta y criterio de aceptación verificable.

---

## Cómo ejecutar este documento

1. Trabaja en una rama propia (`fix/auditoria-*`). **Un commit por hallazgo**,
   con el ID en el mensaje: `fix(ui): move widget mutations to fyne.Do (H-01)`.
2. Los números de línea corresponden a `8545eda`. Si el archivo cambió, guíate
   por la función y por el fragmento citado, no por el número exacto.
3. No cambies comportamiento que no esté listado aquí. Los ítems marcados
   **Decisión** requieren que una persona elija producto/alcance: si no hay
   respuesta, déjalos sin hacer y anótalo.
4. Añade o ajusta tests en el mismo commit del arreglo. El repo exige el
   listón actual (ver línea base).
5. Nada de dependencias nuevas salvo que el hallazgo lo pida. El proyecto evita
   dependencias innecesarias y todo el código de producción es Go estándar +
   Fyne + gopsutil.
6. Migraciones de dependencias (H-01) van en su propio commit, sin mezclar
   arreglos de comportamiento.
7. Si un arreglo rompe la API pública de un paquete interno, actualiza todos
   los llamadores y tests en ese mismo commit; no dejes código muerto.

### Línea base (debe pasar antes y después de cada commit)

```bash
go build ./...
go vet ./...
gofmt -l .                                   # no debe imprimir nada
go test -count=1 ./...
go test -race -count=1 ./...
golangci-lint run ./...                      # hoy: 6 avisos (H-14)
go run golang.org/x/vuln/cmd/govulncheck@latest ./...  # hoy: 0 alcanzables
actionlint .github/workflows/release.yml     # hoy: OK
```

Estado medido en `8545eda`: build, vet, gofmt, tests y `-race` en verde;
`govulncheck` = **0 vulnerabilidades alcanzables** (12 en paquetes importados y
13 en módulos requeridos que el código no llama); `golangci-lint` = 6 avisos;
`actionlint` = OK. Cobertura: `killer` 95,9 %, `scanner` 90,1 %, `ui` 89,4 %,
`cmd/soul` 33,3 %.

---

## Prioridad y dependencias

| ID | Hallazgo | Severidad | Esfuerzo | Depende de |
|----|----------|-----------|----------|------------|
| H-01 | Mutaciones de UI desde goroutines | Alta | M | — |
| H-02 | La escalada a SIGKILL es inalcanzable y el "success" no se verifica | Alta | M | — |
| H-03 | Timeout de escaneo menor que el trabajo que debe hacer | Alta | S | — |
| H-04 | Requisito de memoria/CPU (< 50 MB) incumplido | Media | S (medir) / L (optimizar) | — |
| H-05 | Parseo de rangos de puertos publicado | Media | S | — |
| H-06 | Matching de cgroup frágil (falsos positivos) | Media | S | — |
| H-07 | El diálogo de kill muestra el nombre del proxy, no del servicio | Media | XS | — |
| H-08 | Matar `docker-proxy` no detiene el contenedor | Media | S (aviso) / M (stop) | Decisión |
| H-09 | `Quit`/`StopPolling` bloquean el hilo de UI | Media | S | H-03 |
| H-10 | `bin/soul` (32 MB) y `assets/logo-source.jpeg` versionados | Baja | XS | Decisión |
| H-11 | Deriva entre documentación y código | Baja | S | H-02, H-04 |
| H-12 | CVEs no alcanzables en dependencias de Fyne | Baja | XS | H-01 |
| H-13 | CI solo en tags; sin CI en PRs | Baja | S | — |
| H-14 | 6 avisos de lint | Baja | XS | — |
| H-15 | Falta `LICENSE`; `.atl/` con rutas de máquina versionado | Baja | XS | Decisión |
| H-16 | Sin logging: los errores solo viven en la UI | Baja | S | — |
| H-17 | En Windows `force` se ignora y no está documentado | Baja | XS | — |
| H-18 | Consejo de "correr con sudo" y guarda de PIDs demasiado laxa | Media (seguridad) | S | Decisión |

Esfuerzo: XS < 15 min · S < 1 h · M 1–4 h · L > 4 h.

**Orden recomendado**: H-14 → H-13 → H-03 → H-09 → H-02 → H-07 → H-05 → H-06 →
H-18 → H-16 → H-01 (arrastra H-12) → H-04 → H-11 → H-10 → H-15 → H-17 → H-08.

---

# A. Robustez y funcionalidad

## H-01 · Las actualizaciones de UI se hacen desde goroutines

**Severidad**: Alta · **Esfuerzo**: M

### Evidencia

Fyne exige mutar widgets en el hilo principal. Hoy se hace desde goroutines de
trabajo:

- `internal/ui/app.go:235-291` — goroutine de `Refresh`: `SetScanning`,
  `SetStatus`, `UpdateList` (líneas 256, 263, 267, 283-284).
- `internal/ui/app.go:293-316` — callback de `time.AfterFunc` del debounce:
  `UpdateList`/`SetStatus` (líneas 312-313).
- `internal/ui/app.go:339-386` — goroutine de `ExecuteKill`: `SetStatus`
  (340, 345, 351, 360) y `dialog.ShowConfirm` / `dialog.ShowError`
  (352, 367, 379).
- `internal/ui/view.go:288-302` — `ProcessList.Refresh()` se ejecuta en la
  goroutine que llama a `UpdateList` (es decir, la del escaneo).
- `internal/ui/tray.go:59-101` — `ShowWindow`/`HideWindow`/`RequestFocus`
  llamados desde callbacks del menú de bandeja (systray usa su propio hilo).
- El propio test lo admite: `internal/ui/app_test.go:32-35`, "Fyne widgets
  (which are not goroutine-safe in the general case)".

Fyne 2.5.4 **no** expone `fyne.Do` ni `fyne.DoAndWait` (verificado en el módulo
`fyne.io/fyne/v2@v2.5.4`). Fyne 2.6+ sí: `fyne.io/fyne/v2/thread.go` en
`v2.8.3` define `Do` y `DoAndWait`. La última estable es **v2.8.3** y su
`go.mod` solo exige Go ≥ 1.22, así que es compatible con este proyecto.

### Por qué importa

Repintados inconsistentes, cuelgues y carreras dentro de GTK/GLFW que el
detector de carreras no ve (los widgets no participan en `-race`). Es el
hallazgo de mayor riesgo real.

### Corrección

1. Subir Fyne en un commit aislado:

   ```bash
   go get fyne.io/fyne/v2@v2.8.3
   go mod tidy
   go build ./... && go vet ./... && go test ./...
   ```

   **Verificado durante la auditoría** (en una copia del repo): la subida
   compila, pero `go test ./...` falla **solo** en
   `TestApp_SetsApplicationAndWindowIcon`. Motivo: desde Fyne 2.7
   `desktop.App` exige `SetSystemTrayWindow(fyne.Window)`
   (`fyne.io/fyne/v2@v2.8.3/driver/desktop/app.go`), así que el fake
   `recordingApp` (`internal/ui/app_test.go:108-130`) deja de cumplir la
   interfaz, `configureTray` (`app.go:131-142`) retorna sin configurar la
   bandeja y el test no ve el icono. Añade al fake:

   ```go
   // Fyne >= 2.7 añadió SetSystemTrayWindow a desktop.App.
   func (a *recordingApp) SetSystemTrayWindow(fyne.Window) {}
   ```

   Opcionalmente, en `configureTray`, llama
   `desktopApp.SetSystemTrayWindow(a.window)` para que el clic en el icono de
   bandeja restaure la ventana (comportamiento nuevo de Fyne ≥ 2.7). Con el
   método en el fake, `go test ./...` pasa completo.

   Esto sube de paso `x/net`, `x/sys`, `x/text` y `goldmark`, pero **no**
   elimina todos los CVEs de módulos: ver H-12. Si la subida rompe algo no
   relacionado con la UI, **detente y reporta** en vez de parchear a ciegas.

2. Añadir un *seam* inyectable en `App` para poder testear sin driver real
   (mismo patrón que ya usa el repo con `scanner`, `killer` y `fyne.App`):

   ```go
   // internal/ui/app.go
   type App struct {
       // ...
       runOnMain func(func()) // por defecto fyne.Do
   }
   ```

   Inicialízalo en `NewWithFyneApp` con `runOnMain: fyne.Do`. En los tests,
   asígnalo a `func(f func()) { f() }` para ejecución síncrona y determinista
   (el driver de test de Fyne no garantiza que drene la cola de `fyne.Do` por
   sí solo; no dejes los tests dependiendo de eso).

3. Envolver **todas** las mutaciones de UI. Por ejemplo, en `Refresh`:

   ```go
   procs, err := a.scanner.Scan(ctx)

   a.runOnMain(func() { a.view.SetScanning(false) })

   if err != nil {
       msg := fmt.Sprintf("Scan failed: %v", err)
       a.runOnMain(func() { a.view.SetStatus(msg) })
       return
   }

   filtered := a.state.SetProcesses(procs)
   query := a.state.GetQuery()
   statusMsg := buildStatus(...) // ver H-07: nombre visible, no proc.Name

   a.runOnMain(func() {
       a.view.UpdateList(filtered)
       a.view.SetStatus(statusMsg)
   })
   ```

   El callback del debounce (`app.go:306-314`) se envuelve completo:

   ```go
   a.debounceTimer = time.AfterFunc(DefaultSearchDebounce, func() {
       filtered := a.state.SetQuery(query)
       // ...calcular statusMsg...
       a.runOnMain(func() {
           a.view.UpdateList(filtered)
           a.view.SetStatus(statusMsg)
       })
   })
   ```

4. En `ExecuteKill` (que corre en goroutine) envuelve también los diálogos:

   ```go
   a.runOnMain(func() {
       dialog.ShowConfirm("Termination Failed", msg, func(ok bool) { ... }, a.window)
   })
   ```

   No llames a `fyne.DoAndWait` desde callbacks de UI (riesgo de deadlock);
   `fyne.Do` mantiene el orden FIFO de la cola.

5. En `internal/ui/tray.go`, envuelve `window.Show/Hide/RequestFocus` en el
   mismo seam (`runOnMain`) o documenta y aplica `fyne.Do` directamente en
   `ShowWindow`/`HideWindow`.

6. Actualiza el comentario de `viewer` (`app.go:40-45`) y el de
   `app_test.go:32-35`: el `noopViewer` sigue siendo útil, pero ya no porque
   "no haya forma" de mutar widgets con seguridad.

### Criterio de aceptación

- `grep -nE "a\.view\.|dialog\.Show|\.Refresh\(\)" internal/ui/app.go` → cada
  coincidencia está dentro de un closure pasado a `runOnMain`/`fyne.Do`;
  ninguna llamada directa fuera de él.
- `grep -rn "fyne.io/fyne/v2 v2" go.mod` → `v2.8.3` o superior.
- `go test -race ./...` en verde y `golangci-lint run ./...` sin avisos nuevos.
- Prueba manual (repetir 2-3 minutos): abrir la app, dejar que escanee cada 3 s,
  escribir en el buscador, refrescar y matar un servidor de prueba. Sin
  cuelgues, sin ventana en blanco, sin trazas en la terminal (X11 y Wayland).

---

## H-02 · La escalada a SIGKILL nunca ocurre y el éxito no se verifica

**Severidad**: Alta · **Esfuerzo**: M

### Evidencia

- `internal/ui/app.go:339-386`: solo se ofrece `SIGKILL` si
  `killer.Kill(pid, false)` **devuelve error**. `SIGTERM` a un proceso vivo
  devuelve `nil` aunque el proceso lo ignore, así que la rama es inalcanzable
  en la práctica.
- `internal/ui/app.go:344-347`: si `Kill` devuelve `nil`, la UI dice
  "Successfully terminated" sin comprobar que el proceso murió.
- `internal/killer/killer.go:68-89`: `Kill` solo envía la señal, no espera.
  `IsAlive` existe (`killer.go:92-114`) pero nadie lo usa desde `ui`.
- Incumple `SPEC.md` §5.3: "If process does not terminate within a timeout or
  if requested, send forced termination (SIGKILL)".

### Por qué importa

El caso típico (proceso que ignora `SIGTERM`, servidor trabado, contenedor) deja
el puerto ocupado y la app miente al usuario. Es el flujo central del producto.

### Corrección

1. Extender la interfaz y la implementación de `killer`:

   ```go
   // internal/killer/killer.go
   type Killer interface {
       Kill(pid int32, force bool) error
       WaitForExit(pid int32, timeout time.Duration) bool
   }

   // WaitForExit sondea hasta que el proceso desaparezca o venza el timeout.
   func (k *DefaultKiller) WaitForExit(pid int32, timeout time.Duration) bool {
       deadline := time.Now().Add(timeout)
       for {
           if !k.IsAlive(pid) {
               return true
           }
           if !time.Now().Before(deadline) {
               return false
           }
           time.Sleep(100 * time.Millisecond)
       }
   }
   ```

   **Cuidado con los zombies**: `IsAlive` usa `kill(pid, 0)`, que sigue
   devolviendo éxito para un proceso zombie (`Z`) hasta que lo recoja su padre.
   Sin esto, la app esperaría el timeout completo y ofrecería `SIGKILL` en vano.
   Añade en `signal_unix.go` un helper `isZombie(pid int32) bool` que lea
   `/proc/<pid>/stat` y compruebe que el campo de estado es `Z`, y haz que
   `IsAlive` devuelva `false` en ese caso. En Windows no aplica (el handle ya
   reporta el código de salida): añade un stub `func isZombie(int32) bool {
   return false }` en `signal_windows.go` para que compile en las dos
   plataformas.

2. Reescribir `ExecuteKill` con un timeout de gracia explícito:

   ```go
   const GracefulKillTimeout = 3 * time.Second

   err := a.killer.Kill(proc.PID, false)
   switch {
   case err == nil && a.killer.WaitForExit(proc.PID, GracefulKillTimeout):
       // éxito real: recién ahora decir "terminated" y refrescar
   case err == nil:
       // sigue vivo: ofrecer forzar (SIGKILL)
   case errors.Is(err, killer.ErrPermissionDenied):
       // mensaje neutro, ver H-18 (nada de "run with sudo")
   case errors.Is(err, killer.ErrProcessAlreadyDead):
       // ya estaba muerto
   default:
       // error real
   }
   ```

   Tras `SIGKILL` (`force == true`) vuelve a esperar (`WaitForExit` con 2-3 s) y
   solo entonces reporta éxito; si sigue vivo, muestra error.

3. Añadir tests en `internal/killer` con `WithSignalFunc` simulado:
   - `WaitForExit` devuelve `true` cuando la sonda deja de responder.
   - Devuelve `false` si nunca deja de responder y respeta el timeout (±50 ms).
   - `WaitForExit` no llama a la sonda con `pid <= 1`.

   Y un test de `ui` con un killer falso que exponga `WaitForExit` para cubrir
   las tres ramas (éxito, escalada, error). Al extender la interfaz hay que
   actualizar todos los fakes: `mockKiller` en `internal/ui/app_test.go` y los
   de `internal/killer/*_test.go`.

### Criterio de aceptación

- `grep -n "Successfully terminated" internal/ui/app.go` aparece **después** de
  un `WaitForExit` que devolvió `true`.
- `go test ./internal/killer ./internal/ui` en verde con los tests nuevos.
- Prueba manual con un proceso que ignora `SIGTERM`:

  ```bash
  bash -c 'trap "" TERM; exec sleep 600' &
  echo "PID=$!"
  ```

  En Soul debe aparecer el puerto si el proceso escucha (para esta prueba basta
  con verificar el flujo con un servidor que ignore TERM; si no quieres montar
  uno, usa el test unitario). Al pulsar **Kill** debe aparecer el diálogo de
  escalada y, tras aceptarlo, el proceso debe desaparecer (`kill -0 <PID>` falla)
  antes de mostrarse el mensaje de éxito.

---

## H-03 · El timeout de escaneo es menor que el trabajo que debe hacer

**Severidad**: Alta · **Esfuerzo**: S

### Evidencia

- `internal/ui/app.go:258`: `context.WithTimeout(..., 5*time.Second)` para todo
  el `Scan`.
- `internal/scanner/container.go:55-85`: `List` ejecuta **secuencialmente**
  `docker ps` y `podman ps`, cada uno con `timeout: 3s` (`container.go:58`).
  Peor caso: ~6 s > 5 s.
- Con Docker y Podman instalados, el primer escaneo puede expirar y mostrar
  "Scan failed: context deadline exceeded". El caché de 20 s
  (`container.go:44`) evita que pase en cada refresco, pero el arranque es el
  momento en que el usuario mira.

### Corrección

1. Nombrar el timeout en `app.go` y subirlo por encima del peor caso:

   ```go
   // DefaultScanTimeout bounds a full scan, including container CLI lookups.
   const DefaultScanTimeout = 15 * time.Second
   ```

2. Paralelizar los motores en `commandContainerLister.List`, manteniendo el
   orden determinista de los resultados (importante para tests y para
   `dedupeByID`):

   ```go
   func (l *commandContainerLister) List(ctx context.Context) []ContainerInfo {
       results := make([][]ContainerInfo, len(l.engines))
       seen := make([]bool, len(l.engines))
       var wg sync.WaitGroup

       for i, engine := range l.engines {
           if _, err := l.lookPath(engine); err != nil {
               continue
           }
           seen[i] = true
           wg.Add(1)
           go func(i int, engine string) {
               defer wg.Done()
               cmdCtx, cancel := context.WithTimeout(ctx, l.timeout)
               defer cancel()
               data, err := l.run(cmdCtx, engine, "ps", "--no-trunc", "--format",
                   "{{.ID}}\t{{.Names}}\t{{.Ports}}")
               if err != nil {
                   return // motores ausentes/daemon caído no son error de escaneo
               }
               results[i] = parseContainerListing(engine, string(data))
           }(i, engine)
       }
       wg.Wait()

       var out []ContainerInfo
       for i := range results {
           if seen[i] {
               out = append(out, results[i]...)
           }
       }
       return dedupeByID(out)
   }
   ```

   Peor caso pasa a ~3 s y el escaneo completo cabe en 15 s con margen para
   `gopsutil` y la resolución por proceso.

3. Opcional, solo si tras lo anterior sigue apretado: bajar `l.timeout` a
   1,5-2 s. No lo hagas por defecto: un `docker ps` en frío puede tardar.

4. No toques `snapshot` (`container.go:217-229`) en este ítem. Mantener el
   mutex durante `List` serializa resoluciones concurrentes, pero `App` ya evita
   escaneos concurrentes (`app.go:243-247`); si algún día hay dos llamadores,
   se arregla con doble comprobación del caché.

### Criterio de aceptación

- Test nuevo en `internal/scanner/container_test.go`: dos listers falsos que
  duermen 2 s cada uno; `List(ctx)` debe tardar < 3 s (no ~4 s) y devolver los
  resultados en el orden `docker`, `podman`.
- Test de que `Resolve`/`Scan` respeta la cancelación del contexto (ya hay
  cobertura parcial; añadir el caso de timeout).
- `grep -n "5\*time.Second\|DefaultScanTimeout" internal/ui/app.go` → solo el
  constante nuevo.

---

## H-04 · El requisito de memoria/CPU no se cumple (**Decisión**)

**Severidad**: Media · **Esfuerzo**: S medir / L optimizar

### Evidencia

`SPEC.md` §6 pide "< 50 MB RAM while idle; negligible CPU", y `README.md`
promete "minimal memory footprint (< 50MB idle)". Medición con el binario de
release en Wayland (Fyne 2.5.4, driver GLFW): **RSS ≈ 168 MB, CPU ≈ 4,7 % de un
núcleo en reposo, 27 hilos**.

### Qué hacer (en este orden)

1. Repetir la medición en release, X11 y Wayland, ventana visible y oculta
   (bandeja), anotando RSS, CPU y nº de hilos:

   ```bash
   go build -trimpath -ldflags="-s -w" -o /tmp/soul ./cmd/soul
   /tmp/soul & APP_PID=$!
   sleep 10
   awk '/VmRSS|VmSize|Threads/{print}' /proc/$APP_PID/status
   top -b -n1 -p $APP_PID | tail -1
   kill -TERM $APP_PID
   ```

2. **Decisión**: (a) aceptar el coste real de Fyne/OpenGL y corregir SPEC/README
   con la cifra medida y una nota de por qué (esto es lo que se recomienda:
   bajar de 50 MB con Fyne + driver GL no es realista sin cambiar de toolkit);
   o (b) abrir un trabajo de optimización aparte (perfilar con
   `go tool pprof` añadiendo `net/http/pprof` solo en builds de desarrollo, no
   en release).

3. No metas `pprof` en producción ni "optimizaciones" a ciegas dentro de este
   arreglo. Si eliges (a), el cambio va con H-11.

### Criterio de aceptación

- El número medido queda escrito en `README.md`/`SPEC.md` con la plataforma y
  el commit de medición, **o** existe una tarea separada con el perfilado.
- Nadie vuelve a leer una promesa de 50 MB que el binario no cumple.

---

## H-05 · El parseo de rangos de puertos es incorrecto

**Severidad**: Media · **Esfuerzo**: S

### Evidencia

- `internal/scanner/container.go:107`: `rePublishedPort = regexp.MustCompile("(\\d+)->")`.
- `internal/scanner/container.go:143-155`: captura un único número antes de `->`.
  Para `0.0.0.0:8000-8010->8000-8010/tcp` captura `8010` y registra solo ese
  host port; para `127.0.0.1:8080-8082->80-82/tcp`, solo `8082`.
- Consecuencia: `matchContainerByPort` (`container.go:254-263`) puede asociar el
  puerto equivocado o no asociarlo, y `ContainerInfo.Ports` miente.

### Corrección

Parsear por entrada de mapeo en vez de por regex sobre todo el campo:

```go
// maxPublishedPortRange acota la expansión de rangos (p. ej. 1-65535).
const maxPublishedPortRange = 1000

func parsePublishedPorts(ports string) []int {
    seen := make(map[int]bool)
    var out []int
    add := func(p int) {
        if p > 0 && p <= 65535 && !seen[p] {
            seen[p] = true
            out = append(out, p)
        }
    }

    for _, entry := range strings.Split(ports, ",") {
        entry = strings.TrimSpace(entry)
        arrow := strings.Index(entry, "->")
        if arrow < 0 {
            continue // puerto expuesto sin publicar: no tiene host port
        }
        host := entry[:arrow]
        if i := strings.LastIndex(host, ":"); i >= 0 {
            host = host[i+1:] // quita "0.0.0.0:", "[::]:" o "127.0.0.1:"
        }

        if dash := strings.Index(host, "-"); dash >= 0 {
            lo, errLo := strconv.Atoi(host[:dash])
            hi, errHi := strconv.Atoi(host[dash+1:])
            if errLo != nil || errHi != nil || hi < lo {
                continue
            }
            for p := lo; p <= hi && p-lo < maxPublishedPortRange; p++ {
                add(p)
            }
            continue
        }

        if p, err := strconv.Atoi(host); err == nil {
            add(p)
        }
    }
    return out
}
```

Añade casos a `TestParsePublishedPorts` (`container_test.go:12-38`):
`"0.0.0.0:8000-8010->8000-8010/tcp"` → 8000..8010; `"[::]:8080-8082->80-82/tcp"`
→ 8080,8081,8082; `"80/tcp"` → vacío; duplicados IPv4/IPv6 → sin repetir;
`"0.0.0.0:1-65535->1-65535/tcp"` → no explota (cortado por el tope).

### Criterio de aceptación

- Los tests nuevos pasan y `parsePublishedPorts` sigue sin asignar memoria
  desproporcionada con rangos enormes.
- `matchContainerByPort` encuentra un contenedor por cualquier puerto del rango
  (test de resolver con puerto 8005 y un rango 8000-8010).

---

## H-06 · El matching de contenedor por cgroup produce falsos positivos

**Severidad**: Media · **Esfuerzo**: S

### Evidencia

- `internal/scanner/container.go:110`: `reHexID = [0-9a-f]{12,64}` captura
  **cualquier** secuencia hex de 12-64 caracteres del cgroup.
- `internal/scanner/container.go:158-160, 233-243`: se comparan esos trozos
  contra los IDs de contenedores.
- `internal/scanner/container.go:246-251`: `containerIDMatches` acepta prefijo en
  ambas direcciones sin longitud mínima adicional.

Un `app-<32 hex>.scope` de systemd (o un `session-<32 hex>.scope`) puede
coincidir con el prefijo de un ID de contenedor y mostrar el nombre equivocado.
Es peor que no mostrar nombre: el usuario cree que va a matar un contenedor.

### Corrección

1. Extraer IDs solo de formas conocidas de cgroup (docker, podman/libpod,
   containerd/CRI-O) y del path de cgroup v1:

   ```go
   var reContainerID = regexp.MustCompile(
       `(?:^|[/-])(?:docker|libpod|containerd|cri-containerd|crio)-([0-9a-f]{12,64})(?:\.scope)?(?:/|$)`)
   var reContainerPathID = regexp.MustCompile(`/(?:docker|libpod|containers)/([0-9a-f]{12,64})(?:/|$)`)

   func extractContainerIDs(content string) []string {
       content = strings.ToLower(content)
       var ids []string
       for _, re := range []*regexp.Regexp{reContainerID, reContainerPathID} {
           for _, m := range re.FindAllStringSubmatch(content, -1) {
               if len(m) > 1 {
                   ids = append(ids, m[1])
               }
           }
       }
       return ids
   }
   ```

2. En `matchContainerByCgroup`, priorizar coincidencia exacta del ID completo
   antes que por prefijo (dos pasadas sobre `infos`).

3. Endurecer `containerIDMatches`: exigir `len(candidate) >= 12` y
   `len(full) >= 12` además de la relación de prefijo. Con la extracción nueva
   los IDs ya tienen ≥ 12, pero deja la guarda para no depender del regex.

### Criterio de aceptación

Tests nuevos en `container_test.go`:
- `"0::/user.slice/user-1000.slice/app-<32 hex>.scope"` → **ningún** ID.
- `"0::/user.slice/user-1000.slice/session-<32 hex>.scope"` → **ningún** ID.
- `"/system.slice/docker-<64 hex>.scope"` → ID completo.
- `"0::/user.slice/.../libpod-<64 hex>.scope"` → ID completo (podman rootless).
- `"/kubepods/.../cri-containerd-<64 hex>.scope"` → ID completo.
- El caso de docker cgroup v1 (`/docker/<64 hex>`) sigue funcionando.
- `Resolve` con el `app-<hex>.scope` devuelve `""`, no el nombre de un
  contenedor cualquiera.

---

## H-07 · El diálogo de kill muestra `proc.Name`, no el servicio

**Severidad**: Media · **Esfuerzo**: XS

### Evidencia

- `internal/ui/view.go:220, 334-345`: la tabla usa `ServiceName(p)` (nombre del
  contenedor cuando existe).
- `internal/ui/app.go:324-327`: el diálogo de confirmación interpola
  `proc.Name` (p. ej. `docker-proxy`), incoherente con la fila que el usuario
  acaba de pulsar.
- Lo mismo en los mensajes de estado/errores: `app.go:340, 345, 353, 370, 380`.

### Corrección

1. Usar `ServiceName(proc)` en el diálogo y en los mensajes.
2. Para poder testearlo sin GUI, extrae la construcción del texto a una función
   pura en `view.go` (junto a `ServiceName`):

   ```go
   func KillPrompt(proc model.PortProcess) string {
       return fmt.Sprintf("Are you sure you want to terminate %s (PID %d) listening on port %d?",
           ServiceName(proc), proc.PID, proc.Port)
   }
   ```

   y en `PromptKill`: `dialog.ShowConfirm("Kill Process", KillPrompt(proc), ...)`.

### Criterio de aceptación

- Test de `KillPrompt` con un `PortProcess{Name: "docker-proxy",
  ContainerName: "my-postgres"}` → el texto contiene `my-postgres` y no
  `docker-proxy`.
- `grep -n "proc.Name" internal/ui/app.go` → sin coincidencias en mensajes de
  usuario (o solo donde sea intencional, documentado).

---

## H-08 · Matar el proxy no detiene el contenedor (**Decisión**)

**Severidad**: Media · **Esfuerzo**: S (aviso) / M (detener contenedor)

### Evidencia

`docker-proxy`, `rootlessport`, `conmon`, `slirp4netns` son procesos del host que
reenvían el puerto. `killer.Kill` mata el proxy; el contenedor sigue corriendo y
el puerto puede volver o quedar en un estado raro. `README.md` y el badge de la
tabla prometen identificar el contenedor, pero la acción no lo tiene en cuenta.

### Opciones

- **B (mínima, recomendada para este arreglo)**: mantener el kill actual, pero
  si `proc.IsContainer`, el diálogo de confirmación debe decir que se terminará
  el proceso proxy y que el contenedor seguirá en ejecución, sugiriendo
  `docker stop <nombre>` / `podman stop <nombre>`.
- **A (feature)**: ofrecer un botón/diálogo "Detener contenedor" que ejecute
  `docker stop <nombre>` o `podman stop <nombre>` según `ContainerType`, con
  `exec.CommandContext`, argumentos fijos (sin shell), timeout de 10 s y
  manejo de errores en la UI. Requiere decidir si reemplaza o complementa el
  kill del proxy, y añadir un `ContainerStopper` inyectable + tests.

### Criterio de aceptación (si se elige B)

- Test de la función que construye el mensaje: con `IsContainer: true` y
  `ContainerName: "api"` incluye `api` y la advertencia.
- Prueba manual con Docker (si está disponible): contenedor `nginx` publicando
  8080; el diálogo advierte que el contenedor seguirá vivo.
- No se añade ninguna dependencia ni se ejecuta shell.

---

## H-09 · `Quit`/`StopPolling` bloquean el hilo de UI

**Severidad**: Media · **Esfuerzo**: S

### Evidencia

- `internal/ui/app.go:186-199`: `StopPolling` hace `pollWg.Wait()` y
  `scanWg.Wait()`.
- `internal/ui/app.go:202-222`: `Quit` llama a `StopPolling` de forma síncrona;
  `Quit` se invoca desde el menú de bandeja o el botón de cerrar (hilo de UI).
- El escaneo en curso puede tardar hasta el timeout (5 s hoy, 15 s tras H-03).

### Corrección

Guardar el `cancel` del escaneo y cancelarlo antes de esperar:

```go
// en App
scanCancelMu sync.Mutex
scanCancel   context.CancelFunc
```

En `Refresh`, al crear el contexto:

```go
ctx, cancel := context.WithTimeout(context.Background(), DefaultScanTimeout)
a.scanCancelMu.Lock()
a.scanCancel = cancel
a.scanCancelMu.Unlock()
defer cancel()
```

En `StopPolling`, antes de los `Wait()`:

```go
a.scanCancelMu.Lock()
if a.scanCancel != nil {
    a.scanCancel()
    a.scanCancel = nil
}
a.scanCancelMu.Unlock()
```

`scanner.Scan` ya comprueba `ctx.Err()` en el bucle
(`internal/scanner/scanner.go:100-131`) y `exec.CommandContext` mata las CLIs de
contenedores, así que la espera se acorta de verdad.

### Criterio de aceptación

- Test con un `Scanner` falso que bloquee hasta `<-ctx.Done()`: llamar
  `Refresh()` y luego `Quit()`; `Quit` debe retornar en < 500 ms (hoy tarda el
  timeout completo).
- `go test -race ./internal/ui` en verde.
- Prueba manual: pulsar **Quit** justo al arrancar (mientras el indicador de
  escaneo gira); la ventana debe cerrarse al instante.

---

# B. Higiene del repositorio y documentación

## H-10 · Binario e imagen pesada versionados (**Decisión**)

**Severidad**: Baja · **Esfuerzo**: XS

### Evidencia

`git ls-files` incluye `bin/soul` (**32 MB**, ejecutable) y
`assets/logo-source.jpeg` (**1,4 MB**). `.gitignore` ignora `/soul` pero no
`/bin/`. La historia ya está inflada; un `git rm --cached` no la reduce, pero
evita que siga creciendo.

### Corrección (si se decide dejar de versionar el binario)

```bash
git rm --cached bin/soul
printf '\n# Build output\n/bin/\n' >> .gitignore
```

- `assets/logo-source.jpeg` es arte fuente legítimo: si se quiere conservar,
  déjalo; si molesta, muévelo a `docs/` o a un release, no lo borres sin
  preguntar.
- **No** reescribas historia (`filter-repo`/`BFG`) dentro de esta tarea: es
  destructivo y afecta a todos los clones. Si se quiere, es una tarea aparte.

### Criterio de aceptación

- `git ls-files bin/` vacío y `/bin/` en `.gitignore`.
- `go build -o bin/soul ./cmd/soul` sigue funcionando y `git status` queda
  limpio tras compilar.

---

## H-11 · Deriva entre documentación y comportamiento

**Severidad**: Baja · **Esfuerzo**: S

### Evidencia / correcciones

| Dónde | Dice | Realidad | Cambio |
|---|---|---|---|
| `README.md` (Tech Stack) | "Go 1.20+" | `go.mod`: `go 1.27.1` | "Go 1.27+ (Fyne 2.8 requiere ≥ 1.22)" |
| `SPEC.md` §4 | "Go (v1.20+)" | idem | idem |
| `README.md` (Features) | "Instant fuzzy filtering" | coincidencia por subcadena (`internal/ui/state.go:93-142`) | "case-insensitive substring filtering" |
| `SPEC.md` §5.1 | "polling interval (configurable, e.g. every 3-5s)" | fijo `DefaultPollInterval = 3s` (`app.go:33`) | "every 3 s (`DefaultPollInterval`)" o implementar configuración (feature, no este ítem) |
| `README.md` / `SPEC.md` §6 | "< 50 MB idle" | ~168 MB medidos (H-04) | cifra real + nota |
| `SPEC.md` §5.3 | SIGTERM → timeout → SIGKILL | no implementado (H-02) | actualizar tras H-02 |
| `README.md` (Kill) | "SIGTERM (graceful) and SIGKILL (forced)" | `force` es no-op en Windows (H-17) | añadir nota |
| `PLAN.md` | Fases históricas | ya no es el plan vigente | no hace falta tocarlo; no lo uses como estado actual |

### Criterio de aceptación

- `grep -rn "Go 1.20\|fuzzy" README.md SPEC.md` → sin coincidencias.
- Las afirmaciones de `SPEC.md` §5.3 y §6 coinciden con el código tras H-02 y H-04.
- Ningún documento promete algo que el binario no hace.

---

## H-12 · CVEs no alcanzables en dependencias de Fyne

**Severidad**: Baja · **Esfuerzo**: — (se resuelve con H-01)

### Evidencia

`govulncheck` en `8545eda`: **0 alcanzables**, 12 en paquetes importados y 13 en
módulos requeridos (`golang.org/x/net` v0.25.0, `x/sys` v0.20.0, `goldmark`
v1.7.1, etc., arrastrados por Fyne 2.5.4, de dic-2024). No son explotables
desde Soul porque el código no llama a los símbolos vulnerables.

### Corrección

Medido en una copia del repo (misma máquina, 2026-11-25):

| Paso | Alcanzables | Paquetes importados | Módulos requeridos |
|---|---|---|---|
| Estado actual (`8545eda`) | 0 | 12 | 13 |
| Solo `fyne.io/fyne/v2@v2.8.3` | 0 | 10 | 13 (Fyne fija `x/net v0.35.0`) |
| + `x/net@latest`, `x/sys@latest`, `x/text@latest` | 0 | 0 | 1 (`x/image@v0.43.0`, GO-2026-6222) |
| + `x/image@v0.45.0` | 0 | 0 | 0 |

```bash
# tras H-01
go get golang.org/x/net@latest golang.org/x/text@latest golang.org/x/sys@latest
go get golang.org/x/image@v0.45.0
go mod tidy
go build ./... && go test ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Resultado verificado de esa secuencia: `govulncheck` no reporta nada, build y
tests en verde. Son dependencias indirectas que Fyne solo usa en rutas que Soul
no toca, pero dejarlas al día corta el ruido y evita que un futuro uso las
alcance. Si H-01 se pospone, no fuerces estas versiones sobre Fyne 2.5.4: deja
constancia en el README/issue de que son CVEs en módulos no llamados.

---

## H-13 · CI solo se ejecuta en tags

**Severidad**: Baja · **Esfuerzo**: S

### Evidencia

`.github/workflows/release.yml:3-8`: solo `push` de tags `v*` y
`workflow_dispatch`. Los tests corren en el job de release
(`release.yml:46-47`), pero un PR roto no se detecta hasta la release. Además
`go-version: stable` (`release.yml:27`) no está fijado y las acciones no están
pinneadas por SHA.

### Corrección

Añadir `.github/workflows/ci.yml`:

```yaml
name: CI

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: Install Linux graphics dependencies
        run: sudo apt-get update && sudo apt-get install --yes libgl1-mesa-dev xorg-dev
      - name: Check formatting
        run: test -z "$(gofmt -l .)"
      - name: Vet
        run: go vet ./...
      - name: Test (race)
        run: go test -race ./...
      - name: Vulnerability scan
        run: go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

- En `release.yml`, sustituye `go-version: stable` por
  `go-version-file: go.mod` para que el CI y la release usen la misma versión.
- Pinnear acciones por SHA es recomendable pero requiere consultar los SHA
  actuales; si no se hace, usa versiones mayores estables (`@v4`, `@v5`) como
  hoy.

### Criterio de aceptación

- `actionlint .github/workflows/ci.yml .github/workflows/release.yml` sin
  errores.
- El workflow aparece como check en un PR (no verificable sin remoto; anótalo
  como pendiente de confirmar en GitHub).

---

## H-14 · 6 avisos de lint

**Severidad**: Baja · **Esfuerzo**: XS

### Evidencia

```
internal/killer/killer_unix_test.go:84:7  errcheck  (case <-done:)
internal/killer/killer_unix_test.go:125:7 errcheck  (case <-done:)
internal/scanner/scanner_test.go:20:22    errcheck  (defer listener.Close())
internal/scanner/scanner_test.go:415:31   SA1012    (Scan(nil))
internal/ui/app.go:353:5                  ST1005    (error capitalizado)
internal/ui/app.go:380:5                  ST1005    (error capitalizado)
```

### Corrección

- `killer_unix_test.go:84,125`: `done` es `chan error` y el `select` descarta el
  valor que produce `cmd.Wait()`, por eso `errcheck` se queja. Pasa el canal a
  `chan struct{}` y descarta el error de forma explícita. **No** lo compares con
  `nil`: tras SIGTERM/SIGKILL, `Wait` devuelve un `*exec.ExitError` esperado.

  ```go
  done := make(chan struct{}, 1)
  go func() {
      _ = cmd.Wait()
      done <- struct{}{}
  }()
  ```
- `scanner_test.go:20`: `t.Cleanup(func() { if err := listener.Close(); err != nil { t.Errorf(...) } })`.
- `scanner_test.go:415`: `scanner.Scan(context.Background())` (y ajusta el
  comentario del test si decía que nil es válido).
- `app.go:353,380`: minúscula inicial en el texto del error; si quieres
  mayúscula visual, ponla en el título del diálogo, no en el error.

### Criterio de aceptación

- `golangci-lint run ./...` → `0 issues`.
- `go test ./...` sigue en verde (los cambios son de tests/mensajes).

---

## H-15 · Falta `LICENSE` y `.atl/` es específico de la máquina (**Decisión**)

**Severidad**: Baja · **Esfuerzo**: XS

### Evidencia

- No hay `LICENSE` en la raíz.
- `.atl/skill-registry.md` está versionado y contiene rutas absolutas del home
  (`/home/jesusu/.agents/skills`, etc.), inútiles para otros.

### Corrección

- **Licencia**: decide la persona (MIT, Apache-2.0, propietaria). Si es MIT,
  añade `LICENSE` con "Copyright (c) 2026 <titular>" y una sección en el README.
  No inventes el titular.
- `.atl/`: `git rm --cached -r .atl` y añade `.atl/` a `.gitignore` (es salida
  de una herramienta local). Mantén el archivo en disco.

### Criterio de aceptación

- Existe `LICENSE` con el titular correcto y el README lo menciona, o no se
  tocó (decisión documentada).
- `git ls-files .atl` vacío y `.atl/` ignorado.

---

## H-16 · Sin logging: los errores solo se ven en la GUI

**Severidad**: Baja · **Esfuerzo**: S

### Evidencia

No hay `log`/`log/slog` en el proyecto. Si el escaneo falla o el resolver de
contenedores revienta, el detalle solo aparece en la barra de estado
(`app.go:266-268`) y se pierde. Diagnosticar un bug reportado es imposible.

### Corrección (mínima)

1. Añadir a `App` un logger inyectable:

   ```go
   logger *slog.Logger
   ```

   En `NewWithFyneApp`, por defecto
   `slog.New(slog.NewTextHandler(os.Stderr, nil))`. En tests, un logger a
   `io.Discard` o a un buffer.

2. Loguear solo eventos relevantes, sin ruido por escaneo:

   ```go
   a.logger.Warn("scan failed", "err", err)
   a.logger.Info("kill", "pid", proc.PID, "force", force, "err", err)
   a.logger.Debug("container resolve failed", "pid", pid, "err", err)
   ```

   No loguees cmdlines completos (pueden contener tokens/credenciales de
   procesos ajenos); nombre, PID y puerto bastan.

3. Documenta en el README que los logs van a `stderr` (visibles al lanzar
   `./soul` desde terminal).

### Criterio de aceptación

- Test: con un scanner falso que devuelve error, el buffer del logger contiene
  "scan failed" y el error.
- `golangci-lint` no se queja y `go test ./...` sigue verde.
- Prueba manual: lanzar la app desde la terminal; al forzar un fallo de escaneo
  (p. ej. matar el daemon de Docker mientras se resuelve, o el test unitario)
  aparece una línea en `stderr`.

---

## H-17 · En Windows `force` se ignora y no está documentado

**Severidad**: Baja · **Esfuerzo**: XS

### Evidencia

`internal/killer/signal_windows.go:18-38`: cualquier señal distinta de 0 es
`TerminateProcess`; no existe SIGTERM "suave". `killer.Kill(pid, false)`
termina el proceso igual que `force=true`. El README no lo dice y el flujo de
H-02 (esperar y escalar) no aplica igual en Windows.

### Corrección

- Documentar en `README.md` (sección de kill o notas por plataforma): "En
  Windows no hay SIGTERM; Soul termina el proceso directamente y la opción
  *force* no cambia el resultado. La escalada SIGTERM → SIGKILL de H-02 solo
  aplica a Linux/macOS."
- En `killer.go`, documentar el contrato de la interfaz: `WaitForExit` tras un
  kill en Windows devolverá `true` casi de inmediato.
- **No** reescribas la implementación de Windows para emular `WM_CLOSE`: es una
  feature distinta (cerrar ventanas limpiamente) y no está pedida.

### Criterio de aceptación

- README menciona la diferencia.
- `GOOS=windows go vet ./internal/...` y `GOOS=darwin go vet ./internal/...`
  siguen en verde (el repo ya los soporta).

---

# C. Seguridad

## H-18 · El consejo de "correr con sudo" y la guarda de PIDs (**Decisión**)

**Severidad**: Media · **Esfuerzo**: S

### Evidencia

- `internal/ui/app.go:351-355`: ante `EPERM`, el diálogo sugiere literalmente
  "Run Soul App with elevated permissions (e.g. sudo)". En una app cuyo propósito
  es enviar señales a PIDs arbitrarios, ejecutarla como root amplía el daño
  potencial (incluido un bug propio).
- `internal/killer/killer.go:69-71` y `app.go:319-322`: solo se bloquea
  `PID <= 1`. PID 2 (`kthreadd`) y servicios del sistema quedan a un clic si el
  usuario tiene privilegios. Además nada impide matar el propio Soul si algún
  día aparece listado (hoy no escucha, pero la guarda es barata).
- Fuera de esto, la superficie es sana: no hay `sh -c`, los argumentos de
  `docker`/`podman` son fijos (`container.go:59-61`), no hay secretos y el
  workflow usa permisos mínimos.

### Corrección propuesta

1. Sustituir el texto por uno neutro y correcto:

   ```go
   fmt.Errorf("permission denied: process %s (PID %d) belongs to another user; "+
       "Soul cannot terminate it without those privileges", name, proc.PID)
   ```

   Y en el README: "Soul no está pensado para ejecutarse como root; si un
   proceso es de otro usuario, ciérralo desde su sesión o con `systemctl`."

2. Añadir una guarda barata y explícita en `ui` (no en `killer`, que debe seguir
   siendo una herramienta de bajo nivel):

   ```go
   func isProtectedProcess(pid int32, user string) bool {
       return pid <= 1 ||
           pid == int32(os.Getpid()) ||
           strings.EqualFold(user, "root") && pid < 1000
   }
   ```

   Si `isProtectedProcess` es verdadero, además del `PID <= 1` actual, pide una
   segunda confirmación con texto explícito ("proceso del sistema/root"). No lo
   bloquees por completo: el usuario puede tener motivos (p. ej. su propio
   servidor en un contenedor rootless reportado como root); solo que sea
   consciente.

3. **Decisión**: si se prefiere bloquear del todo los procesos de `root`, hazlo
   y documenta la limitación; no hay una respuesta obvia sin conocer al usuario.

### Criterio de aceptación

- Test unitario de `isProtectedProcess` (PID 0/1/2/propio, usuario root, usuario
  normal).
- Test de que el diálogo de `EPERM` **no** contiene "sudo".
- README con la nota de no ejecutar como root.
- Prueba manual: con un proceso de otro usuario, el mensaje no invita a sudo y
  el flujo no crashea.

---

# D. Lo que ya está bien (no rehacer)

Verificado durante la auditoría; no lo cambies sin motivo:

- **Arquitectura en capas**: `model` ← `scanner`/`killer` ← `ui`, con
  interfaces inyectables (`Scanner`, `Killer`, `ContainerLister`,
  `ContainerNameResolver`, `lifecycleWindow`). Los tests no necesitan display
  gracias a esos seams.
- **Scanner**: deduplicación IPv4/IPv6 por `(port, pid)`, preferencia de `TCP`
  sobre `TCP6`, orden estable, degradación a `[System / Unprivileged]` cuando
  falta permiso (`scanner.go:196-222`).
- **Killer**: clasificación de `ESRCH`/`EPERM`/`EINVAL` en sentinelas
  (`killer.go:127-164`), guarda `PID <= 1`, implementación Windows con
  `OpenProcess`/`TerminateProcess` y `signal 0` para liveness.
- **Contenedores**: `exec.CommandContext` sin shell, timeouts por motor,
  caché con TTL de 20 s, dedupe por ID, tests con cgroups y salidas `ps`
  simuladas.
- **UI**: icono embebido (`internal/ui/resources.go`), tabla con layout
  proporcional, bandeja con `SetCloseIntercept`, debounce de búsqueda.
- **Versión/CI**: versión embebida desde `internal/version/VERSION` y el
  workflow falla si el tag no coincide; build multi-OS con `.app` de macOS.
- **Seguridad de base**: sin secretos, sin `sh -c`, permisos mínimos en CI.
- **Tests**: sockets TCP reales, procesos `sleep`, carreras verdes, cobertura
  ~90 % en los paquetes con lógica.

---

# Anexo A · Matriz criterio → comando

| ID | Verificación principal |
|----|------------------------|
| H-01 | `grep -nE "a\.view\.\|dialog\.Show" internal/ui/app.go` (todas dentro de `runOnMain`); GUI 2-3 min sin cuelgues |
| H-02 | Test de `WaitForExit` + proceso que ignora TERM; "Successfully" solo tras muerte verificada |
| H-03 | Test de `List` con dos motores lentos (< 3 s); `DefaultScanTimeout` en `app.go` |
| H-04 | `/proc/<pid>/status` y `top`; cifra en README/SPEC o tarea aparte |
| H-05 | Tests de `parsePublishedPorts` con rangos/IPv6/duplicados |
| H-06 | Tests de cgroup con `app-<hex>.scope` (no match) y scopes reales (match) |
| H-07 | Test de `KillPrompt` con `ContainerName` |
| H-08 | Test del mensaje y/o prueba con Docker; decisión registrada |
| H-09 | Test de `Quit` < 500 ms con scanner bloqueante |
| H-10 | `git ls-files bin/` vacío; `git status` limpio tras build |
| H-11 | `grep -rn "Go 1.20\|fuzzy" README.md SPEC.md` sin resultados |
| H-12 | `govulncheck` sin hallazgos (0/0/0) tras los bumps |
| H-13 | `actionlint .github/workflows/ci.yml`; check en PR |
| H-14 | `golangci-lint run ./...` → 0 issues |
| H-15 | `LICENSE` existe / `git ls-files .atl` vacío |
| H-16 | Test de logger con error simulado |
| H-17 | README documenta Windows; `GOOS=windows go vet ./internal/...` |
| H-18 | Test de `isProtectedProcess`; diálogo EPERM sin "sudo" |

# Anexo B · Definition of Done global

Un hallazgo está cerrado cuando:

1. El código/tests/docs del commit implementan exactamente lo descrito (ni más
   ni menos).
2. La línea base completa pasa.
3. El criterio de aceptación del hallazgo está cubierto por un test automático
   **o** por un paso manual reproducido y anotado en el PR.
4. Si hubo una decisión de producto, quedó escrita en el PR/issue y aplicada
   de forma coherente en README/SPEC.
5. No se introdujeron dependencias, flags ni configuraciones no pedidas.
