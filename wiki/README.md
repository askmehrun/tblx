# Publishing this wiki to GitHub

A GitHub wiki lives in its own git repository, named `<repo>.wiki.git`.
These pages are written and reviewed here in the main repo, then pushed
to the wiki repo.

## First time

```sh
# 1. Enable the wiki once in the UI:
#    github.com/askmehrun/tblx → Settings → Features → Wikis ✔
#    (or create any page once via the Wiki tab — this creates the wiki repo)

# 2. Clone the empty wiki repo
git clone https://github.com/askmehrun/tblx.wiki.git
cd tblx.wiki

# 3. Copy the pages in (from the tblx checkout)
cp ../tblx/wiki/*.md .
rm README.md                     # keep only the wiki pages

# 4. Push
git add -A
git commit -m "wiki: TBLX spec, library guide, codebase tour, FAQ"
git push
```

## Updating later

```sh
cd tblx.wiki
git pull
cp ../tblx/wiki/{Home,Spec,Library,Extending,Codebase,FAQ,_Sidebar,_Footer}.md .
git add -A && git commit -m "wiki: <what changed>" && git push
```

## Page map

| file | wiki page |
|---|---|
| `Home.md` | landing page (default) |
| `Spec.md` | the complete TBLX specification |
| `Library.md` | using libtblx from Go / C / Python |
| `Extending.md` | adding types, commands, bindings, flag-bit features |
| `Codebase.md` | a tour of every file in all the repos |
| `FAQ.md` | troubleshooting |
| `_Sidebar.md` | navigation sidebar |
| `_Footer.md` | page footer |

Wiki links use the `[[Page Name|Alias]]` syntax — GitHub resolves them
to the wiki's own URLs.
