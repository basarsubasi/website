const dateFormatter = new Intl.DateTimeFormat("tr-TR", {
  day: "numeric",
  month: "long",
  year: "numeric",
});

export const formatPostDate = (value: string) => dateFormatter.format(new Date(value));

export const postParagraphs = (body: string): string[] =>
  body.replace(/\r\n?/g, "\n").split(/\n[\t ]*\n+/).map((paragraph) => paragraph.trim()).filter(Boolean);
