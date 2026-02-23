# Auth Service — Diseño UI: Panel de Administración
**Producto:** Auth Service Admin Portal  
**Versión:** 1.0.0  
**Fecha:** Febrero 2026  
**Cliente:** Sodexo Chile — Equipo de Transformación Digital

---

## 1. Sistema de Diseño

### 1.1 Paleta de Colores Corporativos Sodexo

| Token | Valor | Uso |
|---|---|---|
| `--sodexo-blue` | `#004899` | Color primario, headers, botones principales |
| `--sodexo-blue-light` | `#0066CC` | Hover estados, links activos |
| `--sodexo-blue-dark` | `#003070` | Sidebar activo, énfasis |
| `--sodexo-red` | `#D0021B` | Errores, alertas críticas, acciones destructivas |
| `--sodexo-white` | `#FFFFFF` | Fondos de tarjetas, contenido principal |
| `--sodexo-gray-light` | `#F4F6F9` | Fondo general de la app |
| `--sodexo-gray-mid` | `#E0E5EC` | Bordes, separadores |
| `--sodexo-gray-text` | `#6B7280` | Texto secundario, labels |
| `--sodexo-text-dark` | `#1A1A2E` | Texto principal |
| `--sodexo-success` | `#16A34A` | Badges activos, mensajes de éxito |
| `--sodexo-warning` | `#D97706` | Alertas de bloqueo, sesiones temporales |
| `--sodexo-badge-bg` | `#EFF6FF` | Fondo de badges de roles/permisos |

### 1.2 Tipografía

| Tipo | Fuente | Tamaño | Peso |
|---|---|---|---|
| Título de página | Inter / Roboto | 24px | 700 |
| Título de sección | Inter / Roboto | 18px | 600 |
| Subtítulo | Inter / Roboto | 16px | 600 |
| Cuerpo | Inter / Roboto | 14px | 400 |
| Label/etiqueta | Inter / Roboto | 12px | 500 |
| Código / slug | Mono (JetBrains Mono) | 13px | 400 |

### 1.3 Componentes Base

**Botones:**
- **Primario:** Fondo `--sodexo-blue`, texto blanco, border-radius 6px, padding 10px 20px
- **Secundario:** Borde `--sodexo-blue`, texto `--sodexo-blue`, fondo transparente
- **Peligro:** Fondo `--sodexo-red`, texto blanco (para eliminar / desactivar)
- **Ghost:** Sin borde, texto `--sodexo-gray-text`, solo hover background

**Badges de Estado:**
- `Activo` → fondo `#DCFCE7`, texto `#16A34A`
- `Inactivo` → fondo `#F3F4F6`, texto `#6B7280`
- `Bloqueado` → fondo `#FEF3C7`, texto `#D97706`
- `Sistema` → fondo `#EFF6FF`, texto `#004899`
- `Temporal` → fondo `#FFF7ED`, texto `#EA580C`

**Inputs:**
- Border `--sodexo-gray-mid`, border-radius 6px
- Focus: borde `--sodexo-blue`, box-shadow sutil azul
- Error: borde `--sodexo-red`

---

## 2. Layout General de la Aplicación

```
┌─────────────────────────────────────────────────────────────────────┐
│  TOPBAR                                                             │
│  [🔷 Sodexo Auth Admin]          [🔔 Alerts]  [👤 Admin ▾]  [⚙️]  │
├──────────────┬──────────────────────────────────────────────────────┤
│              │                                                      │
│  SIDEBAR     │   MAIN CONTENT AREA                                  │
│  (240px)     │                                                      │
│              │  ┌ Breadcrumb ─────────────────────────────────────┐ │
│  🏠 Dashboard│  │ Inicio > Usuarios > Juan Pérez                  │ │
│              │  └────────────────────────────────────────────────┘ │
│  👥 Usuarios │                                                      │
│              │  ┌ Page Title ─────────────────────────────────────┐ │
│  📱 Aplic.   │  │ [Título] [Subtítulo]           [Acción primaria]│ │
│              │  └────────────────────────────────────────────────┘ │
│  🎭 Roles    │                                                      │
│              │  ┌ Content ───────────────────────────────────────┐ │
│  🔑 Permisos │  │                                                 │ │
│              │  │                                                 │ │
│  🏢 CeCos    │  │                                                 │ │
│              │  └────────────────────────────────────────────────┘ │
│  📋 Auditoría│                                                      │
│              │                                                      │
│  ─────────── │                                                      │
│  ⚙️ Config   │                                                      │
│  🔓 Salir    │                                                      │
└──────────────┴──────────────────────────────────────────────────────┘
```

### Topbar (altura: 64px)
- **Fondo:** `--sodexo-blue` (`#004899`)
- **Logo izquierda:** Isotipo Sodexo blanco + texto "Auth Admin" en blanco
- **Derecha:** 
  - Ícono de notificaciones (campana) con badge contador de alertas activas
  - Avatar + nombre del admin autenticado + chevron (dropdown: "Mi perfil", "Cambiar contraseña", "Cerrar sesión")
  - Ícono de configuración rápida

### Sidebar (ancho: 240px, colapsable a 64px)
- **Fondo:** `#1A2B4A` (azul muy oscuro, derivado del brand)
- **Item activo:** Fondo `--sodexo-blue`, borde izquierdo 3px blanco
- **Item hover:** Fondo `rgba(255,255,255,0.08)`
- **Texto:** Blanco con opacidad 0.85
- **Texto activo:** Blanco opacidad 1.0

**Ítems del menú:**

```
🏠  Dashboard
─────────────────
👥  Usuarios
📱  Aplicaciones
🎭  Roles
🔑  Permisos
🏢  Centros de Costo
─────────────────
📋  Auditoría
─────────────────
⚙️  Configuración
```

---

## 3. Página: Login

**Ruta:** `/login`  
**Descripción:** Pantalla de autenticación inicial. Muestra cambio de contraseña obligatorio si `must_change_password: true`.

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│                    [Fondo: #F4F6F9 con patrón sutil]               │
│                                                                     │
│            ┌─────────────────────────────────────────┐             │
│            │                                         │             │
│            │     🔷  [LOGO SODEXO]                   │             │
│            │      Auth Service Admin                 │             │
│            │                                         │             │
│            │  ──────────────────────────────────     │             │
│            │                                         │             │
│            │  Usuario                                │             │
│            │  ┌───────────────────────────────────┐  │             │
│            │  │ 👤  jperez                         │  │             │
│            │  └───────────────────────────────────┘  │             │
│            │                                         │             │
│            │  Contraseña                             │             │
│            │  ┌───────────────────────────────────┐  │             │
│            │  │ 🔒  ••••••••••                  👁 │  │             │
│            │  └───────────────────────────────────┘  │             │
│            │                                         │             │
│            │  ┌───────────────────────────────────┐  │             │
│            │  │         Iniciar Sesión             │  │             │
│            │  │       [BOTÓN AZUL SODEXO]          │  │             │
│            │  └───────────────────────────────────┘  │             │
│            │                                         │             │
│            │  ⚠ [Mensaje de error si corresponde]   │             │
│            │                                         │             │
│            │  ─────────────────────────────────────  │             │
│            │  🔒 Acceso restringido a administradores│             │
│            │     autorizados de Sodexo Chile.        │             │
│            │                                         │             │
│            └─────────────────────────────────────────┘             │
│                                                                     │
│                    v1.0.0 · Auth Service · Sodexo                  │
└─────────────────────────────────────────────────────────────────────┘
```

**Estados manejados:**
- Error `INVALID_CREDENTIALS` → alerta roja inline: *"Usuario o contraseña incorrectos"*
- Error `ACCOUNT_LOCKED` → alerta naranja: *"Cuenta bloqueada. Contacta a un administrador."*
- Error `ACCOUNT_INACTIVE` → alerta gris: *"Cuenta inactiva."*
- `must_change_password: true` → redirige automáticamente al modal de **cambio de contraseña obligatorio**

**Modal: Cambio de Contraseña Obligatorio**
```
┌───────────────────────────────────────────────┐
│  🔐 Cambio de Contraseña Requerido            │
│  ─────────────────────────────────────────    │
│  Por seguridad debes cambiar tu contraseña    │
│  antes de continuar.                          │
│                                               │
│  Contraseña actual                            │
│  [••••••••••                               ]  │
│                                               │
│  Nueva contraseña                             │
│  [••••••••••                               ]  │
│  ✅ Mín. 10 chars  ✅ Mayúscula  ✅ Número     │
│  ✅ Símbolo                                   │
│                                               │
│  Confirmar nueva contraseña                   │
│  [••••••••••                               ]  │
│                                               │
│  [      Cambiar Contraseña (Azul)          ]  │
└───────────────────────────────────────────────┘
```

---

## 4. Página: Dashboard

**Ruta:** `/dashboard`  
**Descripción:** Vista general con métricas del sistema, alertas activas y actividad reciente.

```
┌────────────────────────────────────────────────────────────────────┐
│  Dashboard                                    Actualizado hace 2m  │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌───────────┐ │
│  │  👥 Usuarios │ │  📱 Apps     │ │  🎭 Roles    │ │  🔑 Perm. │ │
│  │     1,847    │ │      12      │ │      48      │ │    234    │ │
│  │  ↑ 12 nuevos │ │  11 activas  │ │  46 activos  │ │ 4 apps   │ │
│  │  esta semana │ │              │ │              │ │           │ │
│  └──────────────┘ └──────────────┘ └──────────────┘ └───────────┘ │
│                                                                    │
│  ┌─────────────────────────────────┐ ┌────────────────────────────┐│
│  │  🚨 Alertas del Sistema         │ │  📊 Actividad (7 días)     ││
│  │  ─────────────────────────────  │ │                            ││
│  │  ⚠ 3 cuentas bloqueadas         │ │  [Gráfico de líneas:       ││
│  │    [Ver usuarios →]             │ │   Logins exitosos vs        ││
│  │                                 │ │   fallidos por día]        ││
│  │  ⏰ 5 roles temporales          │ │                            ││
│  │    expiran en < 24h             │ │   Logins: ████ 1,234       ││
│  │    [Ver asignaciones →]         │ │   Fallidos: ▓▓ 89          ││
│  │                                 │ │   Bloqueos: ▒ 3            ││
│  │  ℹ 2 usuarios sin roles         │ │                            ││
│  │    asignados en la última       │ │                            ││
│  │    semana                       │ │                            ││
│  └─────────────────────────────────┘ └────────────────────────────┘│
│                                                                    │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │  📋 Actividad Reciente de Auditoría                            ││
│  │  ─────────────────────────────────────────────────────────     ││
│  │  EVENTO               ACTOR          OBJETIVO      HACE        ││
│  │  USER_ROLE_ASSIGNED    admin          jperez        5 min       ││
│  │  USER_CREATED          admin          mfernandez    12 min      ││
│  │  AUTH_ACCOUNT_LOCKED   sistema        rsanchez      1 hora      ││
│  │  ROLE_CREATED          admin          Bodeguero     2 horas     ││
│  │  USER_UNLOCKED         admin          cgomez        3 horas     ││
│  │                                          [Ver todos los logs →] ││
│  └─────────────────────────────────────────────────────────────────┘│
└────────────────────────────────────────────────────────────────────┘
```

---

## 5. Módulo: Usuarios

### 5.1 Lista de Usuarios

**Ruta:** `/usuarios`

```
┌────────────────────────────────────────────────────────────────────┐
│  Usuarios                                    [+ Nuevo Usuario]     │
│  Gestión de cuentas de usuario del sistema                         │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│  🔍 [Buscar por nombre, usuario o email...]    [Filtros ▾]         │
│                                                                    │
│  Filtros activos: [Estado: Activo ✕] [App: hospitality-app ✕]      │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ ☐ │ USUARIO         │ EMAIL              │ ESTADO  │ ROLES   │  │
│  │─────────────────────────────────────────────────────────────│  │
│  │ ☐ │ 👤 jperez       │ jperez@sodexo.com  │ ●Activo │ 2 roles │  │
│  │   │   Juan Pérez    │                    │         │ ↗       │  │
│  │─────────────────────────────────────────────────────────────│  │
│  │ ☐ │ 👤 rsanchez     │ rsanch@sodexo.com  │ ⚠Bloq.  │ 1 rol   │  │
│  │   │   Rosa Sánchez  │                    │         │ ↗       │  │
│  │─────────────────────────────────────────────────────────────│  │
│  │ ☐ │ 👤 mfernandez   │ mfer@sodexo.com    │ ●Activo │ 3 roles │  │
│  │   │   Mario Fdez.   │                    │         │ ↗       │  │
│  │─────────────────────────────────────────────────────────────│  │
│  │ ☐ │ 👤 cgomez       │ cgomez@sodexo.com  │ ○Inact. │ 0 roles │  │
│  │─────────────────────────────────────────────────────────────│  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  Mostrando 1-20 de 1,847   [< Ant]  1  2  3 ··· 93  [Sig >]       │
└────────────────────────────────────────────────────────────────────┘
```

**Columnas:** Checkbox, Avatar+Nombre+Username, Email, Último Login, Estado (badge), Roles asignados (contador con tooltip), Acciones (ícono: Ver, Editar, más...).

**Acciones en fila (menú contextual `···`):**
- Ver detalle
- Editar usuario
- Asignar roles
- Asignar CeCos
- Asignar permisos especiales
- Resetear contraseña
- Desbloquear cuenta *(solo si está bloqueado)*
- Desactivar cuenta

**Filtros disponibles (panel lateral):**
- Estado: Activo / Inactivo / Bloqueado
- Aplicación
- Tiene roles / Sin roles
- Último login (rango de fechas)

---

### 5.2 Formulario: Crear / Editar Usuario

**Ruta:** `/usuarios/nuevo` | `/usuarios/:id/editar`

```
┌────────────────────────────────────────────────────────────────────┐
│  ← Usuarios   /  Nuevo Usuario                                     │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Información Básica                                          │  │
│  │  ─────────────────────────────────────────────────────────   │  │
│  │  ┌────────────────────────┐  ┌─────────────────────────────┐ │  │
│  │  │  Nombre de usuario *   │  │  Email *                    │ │  │
│  │  │  [jperez             ] │  │  [jperez@sodexo.com       ] │ │  │
│  │  └────────────────────────┘  └─────────────────────────────┘ │  │
│  │                                                              │  │
│  │  ┌────────────────────────┐  ┌─────────────────────────────┐ │  │
│  │  │  Contraseña inicial *  │  │  Estado                     │ │  │
│  │  │  [••••••••••       👁] │  │  [● Activo               ▾] │ │  │
│  │  └────────────────────────┘  └─────────────────────────────┘ │  │
│  │                                                              │  │
│  │  ☑ Forzar cambio de contraseña en primer login              │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Política de contraseña                                      │  │
│  │  ○ Mínimo 10 caracteres    ○ Al menos 1 mayúscula           │  │
│  │  ○ Al menos 1 número       ○ Al menos 1 símbolo             │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│                    [Cancelar]  [Crear Usuario (Azul)]              │
└────────────────────────────────────────────────────────────────────┘
```

---

### 5.3 Detalle de Usuario

**Ruta:** `/usuarios/:id`

Layout de **3 secciones en pestañas:**

```
┌────────────────────────────────────────────────────────────────────┐
│  ← Usuarios                                                        │
│                                                                    │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  👤  Juan Pérez                     ●Activo                 │   │
│  │      jperez  ·  jperez@sodexo.com                          │   │
│  │      Último login: hace 2 horas                            │   │
│  │      Creado: 15 Ene 2026  ·  Por: admin                   │   │
│  │                                                             │   │
│  │  [Editar]  [Reset Contraseña]  [Desactivar]               │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                    │
│  [ Roles ] [ CeCos ] [ Permisos Especiales ] [ Auditoría ]         │
│  ─────────────────────────────────────────────────────────────     │
│                                                                    │
│  ── TAB: Roles ──────────────────────────────────────────────────  │
│                                                                    │
│  [+ Asignar Rol]                                                   │
│                                                                    │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ ROL               APP              VIGENCIA        ACCIÓN   │   │
│  │ chef              hospitality-app  Permanente      [Revocar] │   │
│  │ supervisor        hospitality-app  Hasta 01/03/26  [Revocar] │   │
│  │                                   ⏰ TEMPORAL               │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                    │
│  ── TAB: CeCos ──────────────────────────────────────────────────  │
│                                                                    │
│  [+ Asignar CeCo]                                                  │
│                                                                    │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ CÓDIGO   NOMBRE              APP              VIGENCIA      │   │
│  │ CC001    Casino Central      hospitality-app  Permanente    │   │
│  │ CC002    Comedor Ejecutivo   hospitality-app  Permanente    │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                    │
│  ── TAB: Permisos Especiales ────────────────────────────────────  │
│                                                                    │
│  [+ Asignar Permiso Especial]                                      │
│  ℹ Estos permisos se otorgan directamente al usuario,             │
│    independientemente de sus roles.                               │
│                                                                    │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ PERMISO                   APP              VIGENCIA  ACCIÓN │   │
│  │ reports.special.export    hospitality-app  Perm.    [Revocar]│   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                    │
│  ── TAB: Auditoría ──────────────────────────────────────────────  │
│                                                                    │
│  (Historial de eventos relacionados con este usuario)              │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ EVENTO              ACTOR    DETALLE             FECHA       │   │
│  │ USER_ROLE_ASSIGNED  admin    chef asignado       20/02/26   │   │
│  │ AUTH_LOGIN_SUCCESS  sistema  IP: 10.0.0.5        20/02/26   │   │
│  └─────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────┘
```

---

### 5.4 Modal: Asignar Rol a Usuario

```
┌──────────────────────────────────────────────────┐
│  Asignar Rol a jperez                       [✕]  │
│  ──────────────────────────────────────────────  │
│                                                  │
│  Aplicación *                                    │
│  [hospitality-app                            ▾]  │
│                                                  │
│  Rol *                                           │
│  [🔍 Buscar rol...                           ▾]  │
│  ○ chef          ○ bodeguero  ○ supervisor       │
│  ○ auditor       ○ gerente                       │
│                                                  │
│  Vigencia                                        │
│  ● Permanente                                    │
│  ○ Temporal  [Desde: ──/──/────] [Hasta: ──/──/────]│
│                                                  │
│  Otorgado por: admin (automático)                │
│                                                  │
│  [Cancelar]        [Asignar Rol (Azul)]          │
└──────────────────────────────────────────────────┘
```

---

### 5.5 Modal: Asignar Permisos Especiales

```
┌──────────────────────────────────────────────────┐
│  Asignar Permiso Especial a jperez          [✕]  │
│  ──────────────────────────────────────────────  │
│                                                  │
│  Aplicación *                                    │
│  [hospitality-app                            ▾]  │
│                                                  │
│  Permiso *                                       │
│  [🔍 Buscar permiso por código...                ]│
│                                                  │
│  ┌──────────────────────────────────────────────┐│
│  │ reports.special.export  · Exportar reporte   ││
│  │ finance.ceco.write      · Escribir en CeCo   ││
│  │ admin.system.view       · Ver configuración  ││
│  └──────────────────────────────────────────────┘│
│                                                  │
│  Vigencia                                        │
│  ● Permanente                                    │
│  ○ Temporal  [Desde: ──/──/────] [Hasta: ──/──/────]│
│                                                  │
│  [Cancelar]   [Asignar Permiso (Azul)]           │
└──────────────────────────────────────────────────┘
```

---

### 5.6 Modal: Asignar CeCos a Usuario

```
┌──────────────────────────────────────────────────┐
│  Asignar Centros de Costo a jperez          [✕]  │
│  ──────────────────────────────────────────────  │
│                                                  │
│  Aplicación *                                    │
│  [hospitality-app                            ▾]  │
│                                                  │
│  Centros de Costo disponibles                    │
│  [🔍 Buscar por código o nombre...               ]│
│                                                  │
│  ☑ CC001 — Casino Central                        │
│  ☑ CC002 — Comedor Ejecutivo                     │
│  ☐ CC003 — Cafetería Norte                       │
│  ☐ CC004 — Comedor Planta 2                      │
│                                                  │
│  Vigencia                                        │
│  ● Permanente                                    │
│  ○ Temporal  [Desde: ──/──/────] [Hasta: ──/──/────]│
│                                                  │
│  [Cancelar]       [Guardar CeCos (Azul)]         │
└──────────────────────────────────────────────────┘
```

---

## 6. Módulo: Aplicaciones

### 6.1 Lista de Aplicaciones

**Ruta:** `/aplicaciones`

```
┌────────────────────────────────────────────────────────────────────┐
│  Aplicaciones                          [+ Nueva Aplicación]        │
│  Sistemas registrados en el Auth Service                           │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│  🔍 [Buscar aplicación...]                                         │
│                                                                    │
│  ┌─────────┬──────────────────┬────────────────────┬─────────────┐ │
│  │  📱     │  hospitality-app │  ●Activo           │ [···]       │ │
│  │         │  Hospitality App │  12 Roles · 89 Perm│             │ │
│  │         │  24 Usuarios     │  18 CeCos          │             │ │
│  ├─────────┼──────────────────┼────────────────────┼─────────────┤ │
│  │  📱     │  finance-portal  │  ●Activo           │ [···]       │ │
│  │         │  Portal Finanzas │  8 Roles · 45 Perm │             │ │
│  │         │  6 Usuarios      │  34 CeCos          │             │ │
│  ├─────────┼──────────────────┼────────────────────┼─────────────┤ │
│  │  📱     │  system          │  ■Sistema          │ [···]       │ │
│  │         │  Sistema Auth    │  1 Rol · admin     │ (protegido) │ │
│  └─────────┴──────────────────┴────────────────────┴─────────────┘ │
└────────────────────────────────────────────────────────────────────┘
```

---

### 6.2 Formulario: Crear / Editar Aplicación

**Ruta:** `/aplicaciones/nueva` | `/aplicaciones/:id/editar`

```
┌────────────────────────────────────────────────────────────────────┐
│  ← Aplicaciones  /  Nueva Aplicación                               │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Información de la Aplicación                                │  │
│  │  ─────────────────────────────────────────────────────────   │  │
│  │  Nombre *                                                    │  │
│  │  [Hospitality App                                          ] │  │
│  │                                                              │  │
│  │  Slug * (identificador único, solo minúsculas y guiones)     │  │
│  │  [hospitality-app                                          ] │  │
│  │  ℹ Se usa como identificador en los JWT y en las APIs.      │  │
│  │                                                              │  │
│  │  Estado                                                      │  │
│  │  [● Activa                                               ▾] │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Secret Key                                                  │  │
│  │  ─────────────────────────────────────────────────────────   │  │
│  │  [••••••••••••••••••••••••••••••  [👁 Mostrar] [🔄 Rotar]]   │  │
│  │  ⚠ La secret key se usa para que los backends verifiquen    │  │
│  │    tokens. Guárdala de forma segura.                        │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│                  [Cancelar]  [Crear Aplicación (Azul)]             │
└────────────────────────────────────────────────────────────────────┘
```

---

### 6.3 Detalle de Aplicación

**Ruta:** `/aplicaciones/:id`

Pestañas: **Información General** | **Roles** | **Permisos** | **Centros de Costo** | **Usuarios** | **Auditoría**

Cada pestaña muestra la lista del recurso filtrado por esa aplicación, con accesos rápidos para crear nuevos recursos dentro de la aplicación.

---

## 7. Módulo: Roles

### 7.1 Lista de Roles

**Ruta:** `/roles`

```
┌────────────────────────────────────────────────────────────────────┐
│  Roles                                        [+ Nuevo Rol]        │
│  Agrupadores de permisos por aplicación                            │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│  🔍 [Buscar rol...]      Aplicación: [Todas ▾]   Estado: [Todos ▾] │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ NOMBRE          APP               PERMISOS   USUARIOS  EST.  │  │
│  │─────────────────────────────────────────────────────────────│  │
│  │ 🎭 admin        system            Todos       1        ■Sis. │  │
│  │    Administrador del sistema       protegido                  │  │
│  │─────────────────────────────────────────────────────────────│  │
│  │ 🎭 chef         hospitality-app   12          45       ●Act. │  │
│  │    Jefe de cocina                                            │  │
│  │─────────────────────────────────────────────────────────────│  │
│  │ 🎭 bodeguero    hospitality-app    8          23       ●Act. │  │
│  │    Encargado de bodega                                       │  │
│  │─────────────────────────────────────────────────────────────│  │
│  │ 🎭 supervisor   hospitality-app   18           7       ●Act. │  │
│  └──────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────┘
```

---

### 7.2 Formulario: Crear / Editar Rol

**Ruta:** `/roles/nuevo` | `/roles/:id/editar`

```
┌────────────────────────────────────────────────────────────────────┐
│  ← Roles  /  Nuevo Rol                                             │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Información del Rol                                         │  │
│  │  ─────────────────────────────────────────────────────────   │  │
│  │  Nombre *               Aplicación *                         │  │
│  │  [chef                ] [hospitality-app               ▾]   │  │
│  │                                                              │  │
│  │  Descripción                                                 │  │
│  │  [Jefe de cocina con acceso a stock e inventario          ]  │  │
│  │                                                              │  │
│  │  Estado                                                      │  │
│  │  [● Activo                                               ▾] │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Asignar Permisos                                            │  │
│  │  ─────────────────────────────────────────────────────────   │  │
│  │  🔍 [Buscar permiso por código o descripción...]             │  │
│  │                                                              │  │
│  │  Módulo: inventory                                           │  │
│  │  ☑ inventory.stock.read    · Ver stock                      │  │
│  │  ☑ inventory.stock.write   · Modificar stock                │  │
│  │  ☐ inventory.stock.delete  · Eliminar registros de stock    │  │
│  │                                                              │  │
│  │  Módulo: reports                                             │  │
│  │  ☑ reports.monthly.export  · Exportar reporte mensual       │  │
│  │  ☐ reports.special.export  · Exportar reporte especial      │  │
│  │                                                              │  │
│  │  Módulo: finance                                             │  │
│  │  ☐ finance.ceco.read       · Ver centros de costo           │  │
│  │  ☐ finance.ceco.write      · Modificar centros de costo     │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│            [Cancelar]       [Guardar Rol (Azul)]                   │
└────────────────────────────────────────────────────────────────────┘
```

---

### 7.3 Detalle de Rol

**Ruta:** `/roles/:id`

Pestañas: **Permisos asignados** | **Usuarios con este rol** | **Auditoría**

```
┌────────────────────────────────────────────────────────────────────┐
│  ← Roles                                                           │
│                                                                    │
│  🎭 chef  ·  hospitality-app                    [Editar]  [···]    │
│  Jefe de cocina con acceso a stock e inventario                    │
│  ●Activo  ·  12 permisos  ·  45 usuarios asignados                 │
│                                                                    │
│  [ Permisos ] [ Usuarios ] [ Auditoría ]                           │
│  ──────────────────────────────────────────────────────────────    │
│                                                                    │
│  ── TAB: Permisos ──────────────────────────────────────────────   │
│                                                                    │
│  [+ Agregar Permisos]                                              │
│                                                                    │
│  CÓDIGO                     DESCRIPCIÓN           SCOPE    ACCIÓN  │
│  inventory.stock.read       Ver stock             module   [✕]     │
│  inventory.stock.write      Modificar stock       module   [✕]     │
│  reports.monthly.export     Exportar mensual      action   [✕]     │
│                                                                    │
│  ── TAB: Usuarios ──────────────────────────────────────────────   │
│                                                                    │
│  [Buscar usuario...]                                               │
│                                                                    │
│  USERNAME    NOMBRE           VIGENCIA       OTORGADO POR          │
│  jperez      Juan Pérez       Permanente     admin                 │
│  mfernandez  Mario Fernández  Hasta 01/03/26 admin (⏰ TEMPORAL)   │
└────────────────────────────────────────────────────────────────────┘
```

---

## 8. Módulo: Permisos

### 8.1 Lista de Permisos

**Ruta:** `/permisos`

```
┌────────────────────────────────────────────────────────────────────┐
│  Permisos                                    [+ Nuevo Permiso]     │
│  Permisos disponibles por aplicación                               │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│  🔍 [Buscar por código...]   App: [Todas ▾]   Scope: [Todos ▾]     │
│                                                                    │
│  Agrupar por módulo:                                               │
│                                                                    │
│  ▼ inventory  (4 permisos)                                         │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ CÓDIGO                    APP               SCOPE    ROLES   │  │
│  │ inventory.stock.read      hospitality-app   module   3       │  │
│  │ inventory.stock.write     hospitality-app   module   2       │  │
│  │ inventory.stock.delete    hospitality-app   action   1       │  │
│  │ inventory.orders.read     hospitality-app   module   4       │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  ▶ reports  (3 permisos) [Expandir]                                │
│  ▶ finance  (5 permisos) [Expandir]                                │
│  ▶ admin    (2 permisos) [Expandir]                                │
└────────────────────────────────────────────────────────────────────┘
```

---

### 8.2 Formulario: Crear Permiso

**Ruta:** `/permisos/nuevo`

```
┌────────────────────────────────────────────────────────────────────┐
│  ← Permisos  /  Nuevo Permiso                                      │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Información del Permiso                                     │  │
│  │  ─────────────────────────────────────────────────────────   │  │
│  │  Aplicación *                                                │  │
│  │  [hospitality-app                                        ▾] │  │
│  │                                                              │  │
│  │  Código del permiso *  (formato: módulo.recurso.acción)      │  │
│  │  [inventory.stock.read                                     ] │  │
│  │                                                              │  │
│  │  Vista previa: `inventory.stock.read`                        │  │
│  │    Módulo:   inventory                                      │  │
│  │    Recurso:  stock                                          │  │
│  │    Acción:   read                                           │  │
│  │                                                              │  │
│  │  Descripción                                                 │  │
│  │  [Permite ver el inventario de stock actual                ] │  │
│  │                                                              │  │
│  │  Scope *                                                     │  │
│  │  ○ global   ● module   ○ resource   ○ action                │  │
│  │  ℹ Define el alcance de este permiso en el sistema.         │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│              [Cancelar]     [Crear Permiso (Azul)]                 │
└────────────────────────────────────────────────────────────────────┘
```

---

## 9. Módulo: Centros de Costo (CeCo)

### 9.1 Lista de CeCos

**Ruta:** `/cecos`

```
┌────────────────────────────────────────────────────────────────────┐
│  Centros de Costo                            [+ Nuevo CeCo]        │
│  Unidades organizacionales para control de acceso a datos          │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│  🔍 [Buscar por código o nombre...]   App: [Todas ▾]   [Activos ▾] │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ CÓDIGO   NOMBRE              APLICACIÓN          USUARIOS EST│  │
│  │──────────────────────────────────────────────────────────────│  │
│  │ CC001    Casino Central      hospitality-app     12      ●   │  │
│  │ CC002    Comedor Ejecutivo   hospitality-app      8      ●   │  │
│  │ CC003    Cafetería Norte     hospitality-app      5      ●   │  │
│  │ CC004    Comedor Planta 2    hospitality-app      3      ○   │  │
│  │ CC010    Sede Santiago       finance-portal      15      ●   │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  Mostrando 1-20 de 47                    [< Ant] 1  2  3 [Sig >]   │
└────────────────────────────────────────────────────────────────────┘
```

---

### 9.2 Formulario: Crear / Editar CeCo

**Ruta:** `/cecos/nuevo` | `/cecos/:id/editar`

```
┌────────────────────────────────────────────────────────────────────┐
│  ← Centros de Costo  /  Nuevo Centro de Costo                      │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Información del Centro de Costo                             │  │
│  │  ─────────────────────────────────────────────────────────   │  │
│  │  Aplicación *                                                │  │
│  │  [hospitality-app                                        ▾] │  │
│  │                                                              │  │
│  │  Código *              Nombre *                              │  │
│  │  [CC005             ]  [Cafetería Sur                      ] │  │
│  │                                                              │  │
│  │  Estado                                                      │  │
│  │  [● Activo                                               ▾] │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│              [Cancelar]     [Crear CeCo (Azul)]                    │
└────────────────────────────────────────────────────────────────────┘
```

---

## 10. Módulo: Auditoría

### 10.1 Logs de Auditoría

**Ruta:** `/auditoria`

```
┌────────────────────────────────────────────────────────────────────┐
│  Auditoría                                    [📥 Exportar CSV]    │
│  Registro inmutable de todos los eventos de seguridad              │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Filtros                                                     │  │
│  │  ─────────────────────────────────────────────────────────   │  │
│  │  Tipo de evento:  [Todos ▾]     Aplicación: [Todas ▾]        │  │
│  │  Resultado:       [Todos ▾]     Actor:       [Buscar...    ] │  │
│  │  Usuario afectado:[Buscar...  ] Desde: [──/──/────] Hasta:   │  │
│  │                                [──/──/────]  [Aplicar]       │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ EVENTO               ACTOR     OBJETIVO     APP       RESULT │  │
│  │──────────────────────────────────────────────────────────────│  │
│  │ USER_ROLE_ASSIGNED   admin     jperez        hosp-app  ✅    │  │
│  │ 20/02/2026 10:34     IP:10.0.0.1                             │  │
│  │──────────────────────────────────────────────────────────────│  │
│  │ AUTH_LOGIN_FAILED    —         rsanchez      hosp-app  ❌    │  │
│  │ 20/02/2026 10:31     IP:10.0.0.9  · Intento 3/5             │  │
│  │──────────────────────────────────────────────────────────────│  │
│  │ AUTH_ACCOUNT_LOCKED  sistema   rsanchez      hosp-app  ✅    │  │
│  │ 20/02/2026 10:31     Bloqueo automático                      │  │
│  │──────────────────────────────────────────────────────────────│  │
│  │ ROLE_CREATED         admin     supervisor    hosp-app  ✅    │  │
│  │ 20/02/2026 09:15                                             │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  Mostrando 1-20 de 14,892              [< Ant]  1  2  3  [Sig >]   │
└────────────────────────────────────────────────────────────────────┘
```

**Categorías de eventos disponibles en el filtro:**

| Categoría | Eventos |
|---|---|
| Autenticación | LOGIN_SUCCESS, LOGIN_FAILED, LOGOUT, TOKEN_REFRESHED, PASSWORD_CHANGED, PASSWORD_RESET, ACCOUNT_LOCKED |
| Autorización | PERMISSION_GRANTED, PERMISSION_DENIED |
| Usuarios | USER_CREATED, USER_UPDATED, USER_DEACTIVATED, USER_UNLOCKED |
| Roles | ROLE_CREATED, ROLE_UPDATED, ROLE_DELETED, ROLE_PERMISSION_ASSIGNED, ROLE_PERMISSION_REVOKED |
| Asignaciones | USER_ROLE_ASSIGNED, USER_ROLE_REVOKED, USER_PERMISSION_ASSIGNED, USER_PERMISSION_REVOKED, USER_COST_CENTER_ASSIGNED |

---

### 10.2 Detalle de Evento de Auditoría

Al hacer clic en cualquier fila:

```
┌──────────────────────────────────────────────────────────────────┐
│  Detalle del Evento de Auditoría                            [✕]  │
│  ────────────────────────────────────────────────────────────    │
│                                                                  │
│  Evento:    USER_ROLE_ASSIGNED                                   │
│  Resultado: ✅ Exitoso                                           │
│  Fecha:     20/02/2026  10:34:22 UTC                            │
│                                                                  │
│  Actor:     admin (uuid: 3a4f...)   IP: 10.0.0.1                │
│  Usuario:   jperez (uuid: 7b2c...)                              │
│  App:       hospitality-app                                     │
│  Recurso:   user_role · uuid: 9d1a...                           │
│                                                                  │
│  Valor anterior:  —  (nueva asignación)                         │
│                                                                  │
│  Valor nuevo:                                                    │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ {                                                          │ │
│  │   "role": "chef",                                         │ │
│  │   "user": "jperez",                                       │ │
│  │   "valid_from": "2026-02-20T10:34:22Z",                   │ │
│  │   "valid_until": null,                                    │ │
│  │   "granted_by": "admin"                                   │ │
│  │ }                                                          │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  User-Agent:  Mozilla/5.0 (Windows NT 10.0; Win64; x64)...     │
│                                                                  │
│                                      [Cerrar]                   │
└──────────────────────────────────────────────────────────────────┘
```

---

## 11. Funcionalidades Adicionales Identificadas

Las siguientes funcionalidades se derivan del documento de especificaciones técnicas y complementan las requeridas explícitamente:

### 11.1 Panel de Alerta: Cuentas Bloqueadas

Acceso desde el **Dashboard** → alerta de cuentas bloqueadas → lista filtrada de usuarios en estado `BLOQUEADO` con acción rápida de **Desbloquear**.

```
┌────────────────────────────────────────────────────────────────────┐
│  🔓 Cuentas Bloqueadas (3)                          ← Volver       │
├────────────────────────────────────────────────────────────────────┤
│  USERNAME    BLOQUEOS HOY  BLOQ. HASTA    MOTIVO       ACCIÓN      │
│  rsanchez    3 (manual)    Indefinido     5 intentos   [Desbloquear]│
│  lguerrero   1             10:45          5 intentos   [Desbloquear]│
│  pmontoya    2             11:30          5 intentos   [Desbloquear]│
└────────────────────────────────────────────────────────────────────┘
```

### 11.2 Vista: Asignaciones Temporales Próximas a Expirar

Acceso desde el **Dashboard** → alerta de roles temporales por expirar.

```
┌────────────────────────────────────────────────────────────────────┐
│  ⏰ Asignaciones Temporales (próximas 48h)          ← Volver       │
├────────────────────────────────────────────────────────────────────┤
│  USUARIO     ROL/PERMISO         APP            EXPIRA EN  ACCIÓN  │
│  mfernandez  supervisor          hosp-app       3h         [Renovar]│
│  kvargas     bodeguero           hosp-app       18h        [Renovar]│
│  jsoto       reports.export      fin-portal     22h        [Renovar]│
└────────────────────────────────────────────────────────────────────┘
```

### 11.3 Mi Perfil (Admin)

**Ruta:** `/perfil`

Permite al administrador ver su información, cambiar su contraseña y ver sus sesiones activas.

```
┌────────────────────────────────────────────────────────────────────┐
│  Mi Perfil                                                         │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│  👤  admin  ·  admin@sodexo.cl                                     │
│  Último acceso: 20/02/2026  10:30  desde  10.0.0.1                 │
│                                                                    │
│  [ Cambiar Contraseña ]                                            │
│                                                                    │
│  ── Sesiones Activas ──────────────────────────────────────────    │
│  Dispositivo               IP           Tipo    Iniciada  ACCIÓN   │
│  Chrome/Windows 10         10.0.0.1     web     hace 2h   [Actual] │
│  Firefox/macOS             10.0.0.15    web     hace 3d   [Revocar]│
│                                                                    │
│  [Revocar todas las otras sesiones]                                │
└────────────────────────────────────────────────────────────────────┘
```

> **Fundamento:** La tabla `refresh_tokens` almacena `device_info` (user-agent, IP, tipo de cliente), lo que permite mostrar y gestionar sesiones activas. Un admin puede revocar tokens de sesiones sospechosas.

### 11.4 Configuración del Sistema

**Ruta:** `/configuracion`

Muestra información de solo lectura sobre el estado del servicio (salud de la DB, Redis, versión del mapa de permisos). Permite operaciones avanzadas como visualizar la clave pública JWKS y ver la versión del mapa de permisos actual.

```
┌────────────────────────────────────────────────────────────────────┐
│  Configuración del Sistema                                         │
├────────────────────────────────────────────────────────────────────┤
│                                                                    │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  Estado del Servicio                                        │   │
│  │  PostgreSQL:  ● Conectado     Latencia: 2ms                │   │
│  │  Redis:       ● Conectado     Latencia: 0.5ms              │   │
│  │  JWT Keys:    ● Activas       kid: 2026-02-key-01          │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                    │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  Mapa de Permisos                                           │   │
│  │  Versión actual:  a3f8c21d                                  │   │
│  │  Generado:        20/02/2026  10:00:00 UTC                  │   │
│  │  [Ver JWKS (.well-known/jwks.json)]                         │   │
│  │  [Forzar regeneración del mapa]                             │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                    │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  Políticas de Seguridad (solo lectura — se editan en YAML)  │   │
│  │  Intentos máximos:    5           Duración bloqueo: 15 min  │   │
│  │  Bloqueos/día límite: 3           Costo bcrypt:     12      │   │
│  │  TTL access token:    60 min      Historial pwd:    5       │   │
│  │  TTL refresh web:     7 días      TTL refresh mob:  30 días │   │
│  └─────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────┘
```

---

## 12. Flujos de Navegación Clave

```
LOGIN
  └─→ must_change_pwd? ──→ Modal cambio de contraseña ──→ DASHBOARD
  └─→ directo ──→ DASHBOARD

DASHBOARD
  ├─→ Alerta cuentas bloqueadas ──→ Lista usuarios filtrada (bloqueados)
  ├─→ Alerta roles temporales ──→ Lista asignaciones por expirar
  └─→ Log reciente ──→ Auditoría con filtro preseleccionado

USUARIOS
  ├─→ Lista ──→ Crear nuevo
  ├─→ Lista ──→ Click en fila ──→ Detalle usuario
  │     ├─→ Tab Roles ──→ Modal asignar rol (con vigencia)
  │     ├─→ Tab CeCos ──→ Modal asignar CeCos
  │     ├─→ Tab Permisos Especiales ──→ Modal asignar permiso
  │     └─→ Tab Auditoría ──→ Historial del usuario
  └─→ Lista ──→ Menú contextual ──→ Desbloquear / Reset pwd

ROLES
  ├─→ Lista ──→ Crear nuevo (con selector de permisos agrupados)
  └─→ Lista ──→ Click en fila ──→ Detalle rol
        ├─→ Tab Permisos ──→ Modal agregar permisos
        └─→ Tab Usuarios ──→ Ver quién tiene este rol

AUDITORÍA
  └─→ Lista con filtros ──→ Click en fila ──→ Modal detalle con JSON diff
```

---

## 13. Comportamientos Responsivos y de Accesibilidad

- **Breakpoints:** Desktop (≥1280px) | Tablet (768-1279px) | Mobile (no soportado — app admin)
- **Tablet:** Sidebar se colapsa a iconos (64px). Las tablas se vuelven scroll horizontal.
- **Accesibilidad:** 
  - ARIA labels en todos los controles interactivos
  - Contraste mínimo WCAG AA (ratio 4.5:1) verificado contra la paleta Sodexo
  - Navegación completa por teclado (Tab, Enter, Escape para modales)
  - Mensajes de error vinculados a inputs mediante `aria-describedby`

---

## 14. Mensajes de Estado y Notificaciones

**Toast Notifications (esquina superior derecha, 4 segundos):**

| Tipo | Color | Ejemplo |
|---|---|---|
| Éxito | Verde `#16A34A` | "Rol 'chef' asignado a jperez correctamente" |
| Error | Rojo `#D0021B` | "Error al crear usuario: el username ya existe" |
| Advertencia | Naranja `#D97706` | "El rol expirará en menos de 24 horas" |
| Info | Azul `#004899` | "Cargando permisos del mapa..." |

**Confirmaciones destructivas (siempre con modal):**
- Desactivar usuario
- Revocar rol
- Revocar permiso especial
- Eliminar permiso del sistema
- Rotar secret key de aplicación

Ejemplo de modal de confirmación:
```
┌──────────────────────────────────────────────┐
│  ⚠ ¿Confirmar desactivación?           [✕]  │
│  ────────────────────────────────────────    │
│  Vas a desactivar la cuenta de:              │
│  Juan Pérez (jperez)                         │
│                                              │
│  El usuario perderá acceso inmediatamente.   │
│  Esta acción queda registrada en auditoría.  │
│                                              │
│  [Cancelar]    [Desactivar (Rojo)]           │
└──────────────────────────────────────────────┘
```

---

*Fin del documento de diseño UI v1.0.0*  
*Sodexo Chile — Equipo de Transformación Digital — Febrero 2026*
