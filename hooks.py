import os
import shutil


def on_page_markdown(markdown, page, config, files):
    page.meta["_src_uri"] = page.file.src_uri
    return markdown


def on_post_page(output, page, config, **kwargs):
    src_uri = page.meta.get("_src_uri")
    if src_uri:
        tag = '<meta name="doc-source" content="%s">' % src_uri
        output = output.replace("</head>", tag + "</head>", 1)
    return output


def on_post_build(config, **kwargs):
    docs_dir = config["docs_dir"]
    site_dir = config["site_dir"]
    dest_root = os.path.join(site_dir, "_sources")
    for root, _dirs, filenames in os.walk(docs_dir):
        for fn in filenames:
            if not fn.endswith(".md"):
                continue
            src = os.path.join(root, fn)
            rel = os.path.relpath(src, docs_dir)
            dst = os.path.join(dest_root, rel)
            os.makedirs(os.path.dirname(dst), exist_ok=True)
            shutil.copy2(src, dst)
