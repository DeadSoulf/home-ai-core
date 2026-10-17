type SectionGuideProps = {
  title: string;
  description: string;
};

export function SectionGuide({title, description}: SectionGuideProps) {
  return (
    <section className="ux-section-guide" aria-label={title}>
      <span className="ux-guide-symbol" aria-hidden="true">i</span>
      <div>
        <h2>{title}</h2>
        <p>{description}</p>
      </div>
    </section>
  );
}
