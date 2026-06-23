/**
 * Greeting renders a friendly hello.
 * @param props the component props
 * @returns a JSX element
 */
function Greeting(props: { name: string }): JSX.Element {
  return <div className="greeting">Hello, {props.name}!</div>;
}

// Badge renders a small labelled span, as an arrow component.
const Badge = (props: { label: string }): JSX.Element => {
  return <span className="badge">{props.label}</span>;
};

/** Panel wraps children inside a bordered container. */
class Panel {
  /** render returns the panel markup. */
  render(title: string): JSX.Element {
    return (
      <section className="panel">
        <h2>{title}</h2>
      </section>
    );
  }
}
