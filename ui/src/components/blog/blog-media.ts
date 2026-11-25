import { NodeSelection, Plugin } from "@tiptap/pm/state";
import { Node } from "@tiptap/core";
import { mediaURL, mediaWidth, mediaDimension, type MediaNodeType } from "./post-content";

function mediaNode(name: MediaNodeType) {
  const isVideo = name === "blogVideo";
  const tag = isVideo ? "video" : "img";
  const marker = isVideo ? "data-blog-video" : "data-blog-image";
  const placeholder = isVideo ? "Video açıklaması ekleyin…" : "Görsel açıklaması ekleyin…";
  return Node.create({
    name,
    addOptions() {
      return { isDeleted: (_src: string) => false };
    },
    addProseMirrorPlugins() {
      return [new Plugin({
        appendTransaction: (_transactions, _oldState, state) => {
          const tr = state.tr;
          state.doc.descendants((node, pos) => {
            if (node.type.name === name && this.options.isDeleted(node.attrs.src)) {
              tr.delete(tr.mapping.map(pos), tr.mapping.map(pos + node.nodeSize));
            }
          });
          return tr.docChanged ? tr : null;
        },
      })];
    },
    addNodeView() {
      return ({ node, editor, getPos }) => {
        let current = node;
        const dom = document.createElement("figure");
        dom.className = "blog-image";
        dom.setAttribute(marker, "");
        const frame = document.createElement("div");
        frame.className = "blog-image-frame";
        frame.contentEditable = "false";
        const media = document.createElement(tag);
        if (media instanceof HTMLVideoElement) {
          media.controls = true;
          media.playsInline = true;
          media.preload = "metadata";
          media.addEventListener("pointerdown", () => {
            const pos = getPos();
            if (pos !== undefined) editor.view.dispatch(editor.state.tr.setSelection(NodeSelection.create(editor.state.doc, pos)));
          });
        } else {
          media.alt = "";
          media.decoding = "async";
        }
        frame.append(media);
        const caption = document.createElement("figcaption");
        caption.dataset.placeholder = placeholder;
        dom.append(frame, caption);
        const update = () => {
          dom.dataset.width = String(mediaWidth(current.attrs.width));
          frame.style.width = mediaWidth(current.attrs.width) + "%";
          const src = mediaURL(current.attrs.src);
          if (media.getAttribute("src") !== src) media.src = src;
          for (const [name, value] of [["width", current.attrs.intrinsicWidth], ["height", current.attrs.intrinsicHeight]]) {
            const size = mediaDimension(value);
            if (size) media.setAttribute(name, String(size)); else media.removeAttribute(name);
          }
        };
        update();
        return {
          dom, contentDOM: caption,
          stopEvent: event => isVideo && event.target === media,
          update(node) {
            if (node.type.name !== name) return false;
            current = node;
            update();
            return true;
          },
          selectNode() { dom.classList.add("ProseMirror-selectednode"); },
          deselectNode() { dom.classList.remove("ProseMirror-selectednode"); },
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
    parseHTML() {
      return [{
        tag: `figure[${marker}]`, contentElement: "figcaption",
        getAttrs(element) {
          const media = element.querySelector(tag);
          return {
            src: mediaURL(media?.getAttribute("src")), width: mediaWidth(element.dataset.width),
            intrinsicWidth: mediaDimension(media?.getAttribute("width")),
            intrinsicHeight: mediaDimension(media?.getAttribute("height")),
          };
        },
      }];
    },
    renderHTML({ node }) {
      const { src, width, intrinsicWidth, intrinsicHeight } = node.attrs;
      const attributes = {
        src: mediaURL(src), style: `width: ${mediaWidth(width)}%; margin: 0 auto`,
        width: mediaDimension(intrinsicWidth), height: mediaDimension(intrinsicHeight), contenteditable: "false",
        ...(isVideo ? { controls: true, playsinline: true, preload: "metadata" } : { alt: "", decoding: "async" }),
      };
      return ["figure", { [marker]: "", "data-width": mediaWidth(width), class: "blog-image" },
        [tag, attributes], ["figcaption", { "data-placeholder": placeholder }, 0]];
    },
  });
}

export const BlogImage = mediaNode("blogImage");
export const BlogVideo = mediaNode("blogVideo");
