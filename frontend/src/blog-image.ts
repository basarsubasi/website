import { Plugin } from "@tiptap/pm/state";
import { Node } from "@tiptap/core";
import { imageURL, imageWidth, imageDimension } from "./post-content";

export const BlogImage = Node.create({
  name: "blogImage",
  addOptions() {
    return { deleteImage: async (_src: string) => {}, isDeleted: (_src: string) => false };
  },
  addProseMirrorPlugins() {
    return [new Plugin({
      appendTransaction: (_transactions, _oldState, state) => {
        const tr = state.tr;
        state.doc.descendants((node, pos) => {
          if (node.type.name === "blogImage" && this.options.isDeleted(node.attrs.src)) {
            tr.delete(tr.mapping.map(pos), tr.mapping.map(pos + node.nodeSize));
          }
        });
        return tr.docChanged ? tr : null;
      },
    })];
  },
  addNodeView() {
    return ({ node, editor }) => {
      let current = node;
      const dom = document.createElement("figure");
      dom.className = "blog-image";
      dom.dataset.blogImage = "";
      const frame = document.createElement("div");
      frame.className = "blog-image-frame";
      frame.contentEditable = "false";
      const img = document.createElement("img");
      img.alt = "";
      img.decoding = "async";
      const button = document.createElement("button");
      button.type = "button";
      button.className = "blog-image-delete";
      button.textContent = "Görseli sil";
      button.title = "Görseli R2’den kalıcı olarak sil";
      button.onmousedown = event => event.preventDefault();
      button.onclick = async event => {
        event.preventDefault();
        if (!editor.isEditable || button.disabled) return;
        button.disabled = true;
        try { await this.options.deleteImage(current.attrs.src); }
        finally { button.disabled = false; }
      };
      frame.append(img, button);
      const caption = document.createElement("figcaption");
      caption.dataset.placeholder = "Görsel açıklaması ekleyin…";
      dom.append(frame, caption);
      const update = () => {
        dom.dataset.width = String(imageWidth(current.attrs.width));
        frame.style.width = imageWidth(current.attrs.width) + "%";
        img.src = imageURL(current.attrs.src);
        for (const [name, value] of [["width", current.attrs.intrinsicWidth], ["height", current.attrs.intrinsicHeight]]) {
          const size = imageDimension(value);
          if (size) img.setAttribute(name, String(size)); else img.removeAttribute(name);
        }
      };
      update();
      return {
        dom, contentDOM: caption,
        update(node) { if (node.type.name !== "blogImage") return false; current = node; update(); return true; },
        selectNode() { dom.classList.add("ProseMirror-selectednode"); },
        deselectNode() { dom.classList.remove("ProseMirror-selectednode"); },
        stopEvent: event => button.contains(event.target as globalThis.Node),
        ignoreMutation: mutation => mutation.type !== "selection" && !caption.contains(mutation.target),
      };
    };
  },
  group: "block",
  content: "inline*",
  isolating: true,
  addAttributes() {
    return { src: { default: "" }, width: { default: 100 }, intrinsicWidth: { default: null }, intrinsicHeight: { default: null } };
  },
  parseHTML() { return [{ tag: "figure[data-blog-image]", contentElement: "figcaption", getAttrs: element => ({ src: imageURL(element.querySelector("img")?.getAttribute("src")), width: imageWidth(element.getAttribute("data-width")), intrinsicWidth: imageDimension(element.querySelector("img")?.getAttribute("width")), intrinsicHeight: imageDimension(element.querySelector("img")?.getAttribute("height")) }) }]; },
  renderHTML({ node }) {
    return ["figure", { "data-blog-image": "", "data-width": imageWidth(node.attrs.width), class: "blog-image" },
      ["img", { style: `width: ${imageWidth(node.attrs.width)}%; margin: 0 auto`, src: imageURL(node.attrs.src), alt: "", width: imageDimension(node.attrs.intrinsicWidth), height: imageDimension(node.attrs.intrinsicHeight), decoding: "async", contenteditable: "false" }],
      ["figcaption", { "data-placeholder": "Görsel açıklaması ekleyin…" }, 0]];
  },
});
