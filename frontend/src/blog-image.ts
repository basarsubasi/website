import { Node } from "@tiptap/core";
import { imageURL, imageWidth, imageDimension } from "./post-content";

export const BlogImage = Node.create({
  name: "blogImage",
  group: "block",
  content: "inline*",
  isolating: true,
  addAttributes() {
    return { src: { default: "" }, width: { default: 100 }, intrinsicWidth: { default: null }, intrinsicHeight: { default: null } };
  },
  parseHTML() { return [{ tag: "figure[data-blog-image]", contentElement: "figcaption", getAttrs: element => ({ src: imageURL(element.querySelector("img")?.getAttribute("src")), width: imageWidth(element.getAttribute("data-width")), intrinsicWidth: imageDimension(element.querySelector("img")?.getAttribute("width")), intrinsicHeight: imageDimension(element.querySelector("img")?.getAttribute("height")) }) }]; },
  renderHTML({ node }) {
    return ["figure", { "data-blog-image": "", "data-width": imageWidth(node.attrs.width), class: "blog-image", style: `width: ${imageWidth(node.attrs.width)}%` },
      ["img", { src: imageURL(node.attrs.src), alt: "", width: imageDimension(node.attrs.intrinsicWidth), height: imageDimension(node.attrs.intrinsicHeight), decoding: "async", contenteditable: "false" }],
      ["figcaption", { "data-placeholder": "Görsel açıklaması ekleyin…" }, 0]];
  },
});
