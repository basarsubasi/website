import { Node } from "@tiptap/core";
import { imageURL, imageWidth } from "./post-content";

export const BlogImage = Node.create({
  name: "blogImage",
  group: "block",
  content: "inline*",
  isolating: true,
  addAttributes() {
    return { src: { default: "" }, width: { default: 100 } };
  },
  parseHTML() { return [{ tag: "figure[data-blog-image]", contentElement: "figcaption", getAttrs: element => ({ src: imageURL(element.querySelector("img")?.getAttribute("src")), width: imageWidth(element.getAttribute("data-width")) }) }]; },
  renderHTML({ node }) {
    return ["figure", { "data-blog-image": "", "data-width": imageWidth(node.attrs.width), class: "blog-image", style: `width: ${imageWidth(node.attrs.width)}%` },
      ["img", { src: imageURL(node.attrs.src), alt: "", contenteditable: "false" }],
      ["figcaption", { "data-placeholder": "Görsel açıklaması ekleyin…" }, 0]];
  },
});
