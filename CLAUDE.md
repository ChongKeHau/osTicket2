# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

osTicket (v1.18 line), a PHP 8.2–8.4 / MySQL support-ticket system. No Composer, no build step, no
package.json: vendored libraries live under `include/` (mpdf, laminas-mail, pear, htmLawed, etc.)
and are excluded from lint tests via `Test::$third_party_paths` in `setup/test/tests/class.test.php`.
The remote is upstream `osTicket/osTicket`; `develop` is the main branch and `1.NN.x` branches are
release-maintenance lines that get merged back into `develop`.

## Commands

All tooling is the `manage.php` CLI (run `php manage.php --help`; modules are in `include/cli/modules/`).

```bash
# Run a local dev server (PHP built-in server with osTicket's router)
php manage.php serve --host localhost --port 8000

# Deploy the checkout into a web root (add --setup on first install, -v for verbose, -t dry run)
php manage.php deploy --setup /var/www/htdocs/osticket/
php manage.php deploy -v /var/www/htdocs/osticket/

# Other modules: cron, upgrade, i18n, export/import, file (attachment migration), user/org/agent, package
php manage.php file migrate --backend=6 --to=D
```

### Tests

There is no PHPUnit. The test runner is a custom static-analysis suite (syntax, undefined methods,
uninitialized vars, short open tags, var_dump left behind, git conflict markers, signal
connect/send consistency, trailing whitespace, jslint, plus a few unit tests for crypto, mail
parsing, validation).

```bash
cd setup/test
php run-tests.php                 # run every suite
php run-tests.php SyntaxTest      # run one suite, by the class name the test file returns
php -l path/to/file.php           # quick syntax check of one file
```

Each test is `setup/test/tests/test.<name>.php`, defines a class extending `Test` with `test*`
methods, and ends with `return 'ClassName';`. The exit code is the number of failures.
`test.jslint.php` shells out to `jsl`; it will fail-open if that binary is not installed.

## Request flow and architecture

### Bootstrapping

Every web entry point includes `main.inc.php`, which runs `Bootstrap::loadConfig()` (reads
`include/ost-config.php`, git-ignored, created by the installer), `defineTables()` (all
`*_TABLE` constants from `TABLE_PREFIX`), `loadCode()` (core classes), and `connect()`, then
`osTicket::start()` populates the globals `$ost` (system object) and `$cfg` (DB-backed config).
There is no autoloader for core code: each file `require_once`s the `include/class.*.php`
files it depends on (one class family per file, e.g. `class.ticket.php`, `class.dept.php`).
The only autoloader (`osTicket::register_namespace()`, a PSR-0 fallback loader) is registered
for a plugin's `lib/` directory when that plugin is bootstrapped.

Three front doors, each with its own include that layers on top of `main.inc.php`:

| Area | Entry scripts | Per-page include | Globals |
|---|---|---|---|
| Client portal | root `*.php` (`tickets.php`, `open.php`, `view.php`, `kb/`) | `client.inc.php` | `$thisclient` |
| Staff control panel | `scp/*.php` | `scp/staff.inc.php` (admin pages also `scp/admin.inc.php`) | `$thisstaff` |
| API | `api/http.php`, `api/pipe.php`, `api/cron.php` | `api/api.inc.php` | API key auth |

A typical page script (e.g. `scp/departments.php`) handles the POST, sets `$page`, then
`require`s `STAFFINC_DIR.'header.inc.php'`, the view from `include/staff/*.inc.php`, and
`footer.inc.php`. Views are plain PHP templates; reusable fragments/modals live in
`include/staff/templates/*.tmpl.php` and `include/client/templates/`.

### AJAX and API dispatch

`scp/ajax.php`, root `ajax.php`, `apps/dispatcher.php` and `api/http.php` build a URL router
with `patterns()` / `url()` / `url_get()` / `url_post()` / `url_delete()` from
`include/class.dispatcher.php`. Routes point at `'ajax.<area>.php:ClassName'` methods, so
handlers live in `include/ajax.*.php` (extending `AjaxController`) and `include/api.*.php`
(extending `ApiController`). Plugins can add routes by connecting to the `ajax.scp` /
`ajax.client` signals sent with the dispatcher.

CSRF is enforced globally in `client.inc.php` and `staff.inc.php` for state-changing methods;
the token is exposed via the `csrf_token` meta tag consumed by `js/`.

### ORM

`include/class.orm.php` is a Django-inspired ORM. Models extend `VerySimpleModel` and declare a
`static $meta` array with `table`, `pk`, `ordering`, and `joins` (each join gives a
`constraint` mapping local column to `'OtherModel.column'`, optional `null`, `list`, `reverse`).
Query with `Model::objects()->filter([...])->values(...)`, `Model::lookup($id)`,
`Model::create([...])->save()`. `QuerySet` is lazy; `InstrumentedList` backs `list` joins.
Custom-form data (`__cdata` tables) is exposed via `AnnotatedModel` and the dynamic forms system
in `include/class.dynamic_forms.php` / `class.forms.php` (see `setup/doc/forms.md`).
Legacy code still uses raw `db_query()` / `db_input()` helpers from `include/mysqli.php`.

### Signals

Extensibility is a string-named pub/sub: `Signal::send('name', $this, $data)` and
`Signal::connect('name', callable)`. A static test (`test.signals.php`) requires that every
connected signal name is also sent somewhere in the codebase. See `setup/doc/signals.md`.

### Database schema and upgrades

- Fresh-install schema: `setup/inc/streams/core/install-mysql.sql`; its md5 is stored in
  `include/upgrader/streams/core.sig` and used as the `schema_signature`.
- Migrations are "streams" of patches in `include/upgrader/streams/core/` named
  `<from8>-<to8>.patch.sql` (plus optional `.task.php` / `.cleanup.sql`), chained by the
  first 8 chars of the old and new signature. Changing the install SQL means regenerating the
  md5 in `core.sig` and adding a patch from the previous hash. Full details in
  `setup/doc/streams.md`. The web upgrader runs from `scp/upgrade.php`; the CLI from
  `php manage.php upgrade`.
- Default seed data (departments, roles, email templates, help topics, etc.) is YAML under
  `include/i18n/en_US/*.yaml`, loaded at install time.

### Internationalization

All user-visible strings must go through gettext-style wrappers: `__()` (current user locale),
`_S()` (system locale), `_L()` (explicit locale), `_N`/`_NS`/`_NL` for plurals, `_P`/`_NP` for
context. Translations are pure-PHP gettext; language packs are `.phar` files under
`include/i18n/`. Rules are in `setup/doc/i18n.md`.

### Plugins and auth

Plugins (`include/class.plugin.php`) are `.phar` or directories under `include/plugins/`,
bootstrapped by `PluginManager`; they subclass `Plugin`, declare a `PluginConfig`, and hook in
via signals or by registering `AuthenticationBackend` / `StaffAuthenticationBackend` /
`UserAuthenticationBackend` subclasses (`include/class.auth.php`). Files can be stored via
pluggable `FileStorageBackend`s (`include/class.file.php`).

## Conventions

- Files carry the header comment block with `vim: expandtab sw=4 ts=4 sts=4`; four-space indent.
- Includes guard against direct access with `die('kwaheri rafiki!')` when the script name
  matches the file; keep this pattern in new `*.inc.php` files.
- Release notes go in `WHATSNEW.md` (newest version at top); version constants are in
  `bootstrap.php` (`MAJOR_VERSION`, `THIS_VERSION`).
- Security reports go to security@osticket.com (see `SECURITY.md`), not to the public tracker.
